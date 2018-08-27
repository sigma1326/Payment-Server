package main

import (
	models "Payment-Server/database_models"
	pb "Payment-Server/protofiles"

	"fmt"
	"github.com/dgrijalva/jwt-go"
	"github.com/jinzhu/gorm"
	_ "github.com/jinzhu/gorm/dialects/postgres"
	"golang.org/x/net/context"
	"google.golang.org/grpc/reflection"
	"log"
	"math/rand"
	"net"
	"strconv"
	"strings"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	_ "google.golang.org/grpc/credentials"
	"google.golang.org/grpc/metadata"
)

const (
	//port = "192.168.1.5:1313"
	port = ":1313"
)

// customerServer is used to create MoneyTransactionServer.
type customerServer struct{}
type customerV1Server struct{}

var dbPool *gorm.DB

// private type for Context keys
type contextKey int

const (
	clientIDKey contextKey = iota
)

// authenticateAgent check the client credentials
func authenticateClient(ctx context.Context, s *customerServer) (string, error) {
	if md, ok := metadata.FromIncomingContext(ctx); ok {
		clientLogin := strings.Join(md["login"], "")
		clientPassword := strings.Join(md["password"], "")
		if clientLogin != "john" {
			return "", fmt.Errorf("unknown user %s", clientLogin)
		}

		if clientPassword != "doe" {
			return "", fmt.Errorf("bad password %s", clientPassword)
		}
		log.Printf("authenticated client: %s", clientLogin)
		return "42", nil
	}
	return "", fmt.Errorf("missing credentials")
}

// unaryInterceptor calls authenticateClient with current context
func unaryInterceptor(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (interface{}, error) {
	s, ok := info.Server.(*customerServer)
	if !ok {
		return nil, fmt.Errorf("unable to cast server")
	}
	clientID, err := authenticateClient(ctx, s)
	if err != nil {
		return nil, err
	}
	ctx = context.WithValue(ctx, clientIDKey, clientID)
	return handler(ctx, req)
}

func main() {
	//db, err := gorm.Open("mysql", "root:123456789@/payDemo?charset=utf8mb4&parseTime=True&loc=Local")
	db, err := gorm.Open("postgres", "host=localhost port=5432 user=postgres dbname=paydemo password=123456789 sslmode=disable")
	dbPool = db
	defer db.Close()
	defer dbPool.Close()

	db.LogMode(true)

	//create tables in database for mysql
	//db.Set("gorm:table_options", "ENGINE=InnoDB").AutoMigrate(&models.User{})
	//db.Set("gorm:table_options", "ENGINE=InnoDB").AutoMigrate(&models.Card{})
	//db.Set("gorm:table_options", "ENGINE=InnoDB").AutoMigrate(&models.PayCard{})
	//db.Set("gorm:table_options", "ENGINE=InnoDB").AutoMigrate(&models.Bank{})
	//db.Set("gorm:table_options", "ENGINE=InnoDB").AutoMigrate(&models.Transaction{})

	db.AutoMigrate(&models.User{})
	db.AutoMigrate(&models.Card{})
	db.AutoMigrate(&models.PayCard{})
	db.AutoMigrate(&models.Bank{})
	db.AutoMigrate(&models.Transaction{})

	lis, err := net.Listen("tcp", port)
	if err != nil {
		log.Fatalf("Failed to listen: %v", err)
	}

	// Create the TLS credentials
	creds, err := credentials.NewServerTLSFromFile("cert/server.crt", "cert/server.key")
	if err != nil {
		log.Fatalf("could not load TLS keys: %grpcServer", err)
	}

	// Create an array of gRPC options with the credentials
	opts := []grpc.ServerOption{grpc.Creds(creds), grpc.UnaryInterceptor(unaryInterceptor)}
	//opts := grpc.ServerOption(grpc.Creds(creds))

	grpcServer := grpc.NewServer(opts...)
	pb.RegisterCustomerServer(grpcServer, &customerServer{})
	reflection.Register(grpcServer)

	if err := grpcServer.Serve(lis); err != nil {
		log.Fatalf("Failed to serve: %v", err)
	}

}

func (s *customerServer) TransferMoney(ctx context.Context, in *pb.TransferMoneyRequest) (*pb.TransferMoneyResponse, error) {
	log.Printf("customer transferMoney request : %s", in.MobileNumber)
	responseCodes := make([]pb.TransferMoneyResponse_TransferMoneyResponseCode, 0)

	if len(in.Token) > 0 {
		var checkUser models.User
		var checkCard models.Card
		var checkPayCard models.PayCard

		dbPool.Where("phone_number = ?", in.MobileNumber).First(&checkUser)
		if checkUser.PhoneNumber == in.MobileNumber {
			if checkUser.Blocked == false && checkUser.Disabled == false {
				switch in.CardType {
				case pb.CardType_PayCard:
					dbPool.Where("user_id = ?", checkUser.ID).First(&checkPayCard)
					if checkPayCard.UserID != checkUser.ID {
						responseCodes = append(responseCodes, pb.TransferMoneyResponse_WRONG_USER)
					} else {
						if in.Password == checkUser.Password {
							if in.TransferAmount > 0 {
								if checkPayCard.Balance >= in.TransferAmount {
									var dstCard models.Card
									dbPool.Where("number = ?", in.DstCardNumber).First(&dstCard)

									if dstCard.Number == in.DstCardNumber {
										checkPayCard.Balance -= in.TransferAmount
										dstCard.Balance += in.TransferAmount
										err1 := dbPool.Save(&checkPayCard)
										if err1.Error != nil {
											responseCodes = append(responseCodes, pb.TransferMoneyResponse_FAILED)
											log.Printf(err1.Error.Error())
										} else {
											err2 := dbPool.Save(&dstCard)
											if err2.Error != nil {
												responseCodes = append(responseCodes, pb.TransferMoneyResponse_FAILED)
												log.Printf(err2.Error.Error())
											} else {
												err := dbPool.Create(&models.Transaction{
													Amount:                in.TransferAmount,
													SourceUser:            checkUser.ID,
													DestinationCardNumber: in.DstCardNumber,
													TransactionID:         time.Now().String() + ":" + strconv.FormatInt(in.TransferAmount, 10) + ":" + strconv.FormatInt(rand.Int63n(10000000), 10),
												})
												if err.Error != nil {
													responseCodes = append(responseCodes, pb.TransferMoneyResponse_FAILED)
													log.Printf(err.Error.Error())
												} else {
													responseCodes = append(responseCodes, pb.TransferMoneyResponse_SUCCESS)
												}
											}
										}

									} else {
										//not in our db, it's somewhere else
										checkCard.Balance -= in.TransferAmount
										//dstCard.Balance += in.TransferAmount
										err1 := dbPool.Save(&checkCard)
										if err1.Error != nil {
											responseCodes = append(responseCodes, pb.TransferMoneyResponse_FAILED)
											log.Printf(err1.Error.Error())
										} else {
											err := dbPool.Create(&models.Transaction{
												Amount:                in.TransferAmount,
												SourceUser:            checkUser.ID,
												DestinationCardNumber: in.DstCardNumber,
												TransactionID:         time.Now().String() + ":" + strconv.FormatInt(in.TransferAmount, 10) + ":" + strconv.FormatInt(rand.Int63n(10000000), 10),
											})
											if err.Error != nil {
												responseCodes = append(responseCodes, pb.TransferMoneyResponse_FAILED)
												log.Printf(err.Error.Error())
											} else {
												responseCodes = append(responseCodes, pb.TransferMoneyResponse_SUCCESS)
											}
										}
									}
								} else {
									responseCodes = append(responseCodes, pb.TransferMoneyResponse_INSUFFICIENT_BALANCE)
								}
							} else {
								responseCodes = append(responseCodes, pb.TransferMoneyResponse_WRONG_AMOUNT)
							}
						} else {
							responseCodes = append(responseCodes, pb.TransferMoneyResponse_WRONG_PASSWORD)
						}

					}
				case pb.CardType_BankCard:
					dbPool.Where("number = ?", in.SrcCardNumber).First(&checkCard)
					if checkCard.Number != in.SrcCardNumber {
						responseCodes = append(responseCodes, pb.TransferMoneyResponse_INVALID_SRC_CARD)
					} else {
						if in.Password == "123456" {
							if in.TransferAmount > 0 {
								log.Printf("amount :%v", in.TransferAmount)
								if checkCard.Balance >= in.TransferAmount {
									var dstCard models.Card
									dbPool.Where("number = ?", in.DstCardNumber).First(&dstCard)

									if dstCard.Number == in.DstCardNumber {
										checkCard.Balance -= in.TransferAmount
										dstCard.Balance += in.TransferAmount
										err1 := dbPool.Save(&checkCard)
										if err1.Error != nil {
											responseCodes = append(responseCodes, pb.TransferMoneyResponse_FAILED)
											log.Printf(err1.Error.Error())
										} else {
											err2 := dbPool.Save(&dstCard)
											if err2.Error != nil {
												responseCodes = append(responseCodes, pb.TransferMoneyResponse_FAILED)
												log.Printf(err2.Error.Error())
											} else {
												err := dbPool.Create(&models.Transaction{
													Amount:                in.TransferAmount,
													SourceUser:            checkUser.ID,
													SourceCardNumber:      checkCard.Number,
													DestinationCardNumber: in.DstCardNumber,
													TransactionID:         time.Now().String() + ":" + strconv.FormatInt(in.TransferAmount, 10) + ":" + strconv.FormatInt(rand.Int63n(10000000), 10),
												})
												if err.Error != nil {
													responseCodes = append(responseCodes, pb.TransferMoneyResponse_FAILED)
													log.Printf(err.Error.Error())
												} else {
													responseCodes = append(responseCodes, pb.TransferMoneyResponse_SUCCESS)
												}
											}
										}

									} else {
										//not in our db, it's somewhere else
										checkCard.Balance -= in.TransferAmount
										//dstCard.Balance += in.TransferAmount
										err1 := dbPool.Save(&checkCard)
										if err1.Error != nil {
											responseCodes = append(responseCodes, pb.TransferMoneyResponse_FAILED)
											log.Printf(err1.Error.Error())
										} else {
											err := dbPool.Create(&models.Transaction{
												Amount:                in.TransferAmount,
												SourceUser:            checkUser.ID,
												DestinationCardNumber: in.DstCardNumber,
												TransactionID:         time.Now().String() + ":" + strconv.FormatInt(in.TransferAmount, 10) + ":" + strconv.FormatInt(rand.Int63n(10000000), 10),
											})
											if err.Error != nil {
												responseCodes = append(responseCodes, pb.TransferMoneyResponse_FAILED)
												log.Printf(err.Error.Error())
											} else {
												responseCodes = append(responseCodes, pb.TransferMoneyResponse_SUCCESS)
											}
										}
									}
								} else {
									log.Printf("%v", in.TransferAmount)
									responseCodes = append(responseCodes, pb.TransferMoneyResponse_INSUFFICIENT_BALANCE)
								}
							} else {
								responseCodes = append(responseCodes, pb.TransferMoneyResponse_WRONG_AMOUNT)
							}
						} else {
							responseCodes = append(responseCodes, pb.TransferMoneyResponse_WRONG_PASSWORD)
						}
					}
				default:

				}

			} else {
				if checkUser.Blocked {
					responseCodes = append(responseCodes, pb.TransferMoneyResponse_MOBILE_BLOCKED)
				}
				if checkUser.Disabled {
					responseCodes = append(responseCodes, pb.TransferMoneyResponse_DISABLED)
				}
			}
		} else {
			responseCodes = append(responseCodes, pb.TransferMoneyResponse_WRONG_USER)
		}

	} else {
		responseCodes = append(responseCodes, pb.TransferMoneyResponse_WRONG_TOKEN)
	}
	log.Printf("amount :%v", responseCodes)

	return &pb.TransferMoneyResponse{
		ResponseCodes: responseCodes,
	}, nil
}

func (s *customerServer) CheckMobile(ctx context.Context, in *pb.CheckMobileRequest) (*pb.CheckMobileResponse, error) {
	log.Printf("customer check mobile request : %s", in.MobileNumber)
	responseCodes := make([]pb.CheckMobileResponse_CheckMobileResponseCode, 0)
	msg := ""

	// Set custom claims
	smsClaims := &models.SmsCodeClaims{
		PhoneNumber: in.MobileNumber,
		SmsCode:     rand.Int31n(1000000),
		StandardClaims: jwt.StandardClaims{
			Issuer:    "demo-project",
			Subject:   in.MobileNumber,
			ExpiresAt: time.Now().Add(time.Hour * 24).Unix(),
			NotBefore: time.Now().Unix(),
		},
	}

	// Create token with claims
	smsToken := jwt.NewWithClaims(jwt.SigningMethodHS256, smsClaims)

	// Generate encoded token and send it as response.
	tokenString, err := smsToken.SignedString([]byte("secret"))
	if err != nil {
		log.Fatal(err)
	}

	var user models.User

	dbPool.Where("phone_number = ?", in.MobileNumber).First(&user)

	if user.PhoneNumber == in.MobileNumber {
		msg = "کاربر در سیستم موجود است"
		responseCodes = append(responseCodes, pb.CheckMobileResponse_REGISTERED)
		if user.Blocked {
			responseCodes = append(responseCodes, pb.CheckMobileResponse_MOBILE_BLOCKED)
			msg = "کاربر مسدود شده است"
		}
		if user.Disabled {
			msg = "کاربر غیر فعال شده است"
			responseCodes = append(responseCodes, pb.CheckMobileResponse_DISABLED)
		}
		tokenString = ""
	} else {
		msg = "کاربر در سیستم موجود نیست"
		responseCodes = append(responseCodes, pb.CheckMobileResponse_UNREGISTERED)
	}

	response := &pb.CheckMobileResponse{
		Message:       msg,
		Token:         tokenString,
		ResponseCodes: responseCodes,
	}
	return response, nil
}

func (s *customerServer) CheckLogin(ctx context.Context, in *pb.CheckLoginRequest) (*pb.CheckLoginResponse, error) {
	log.Printf("customer login request : %s", in.MobileNumber)
	responseCodes := make([]pb.CheckLoginResponse_CheckLoginResponseCode, 0)
	msg := ""

	var tokenString string

	var user models.User

	if len(in.MobileNumber) == 12 {
		dbPool.Where("phone_number = ?", in.MobileNumber).First(&user)

		if user.PhoneNumber == in.MobileNumber {
			// Set custom claims
			appClaims := &models.AppClaim{
				PhoneNumber: in.MobileNumber,
				UserID:      user.ID,
				StandardClaims: jwt.StandardClaims{
					Issuer:    "demo-project",
					Subject:   in.MobileNumber,
					ExpiresAt: time.Now().Add(time.Hour * 24).Unix(),
					NotBefore: time.Now().Unix(),
				},
			}

			// Create token with claims
			appToken := jwt.NewWithClaims(jwt.SigningMethodHS256, appClaims)

			// Generate encoded token and send it as response.
			tokenString1, err := appToken.SignedString([]byte("secret"))
			if err != nil {
				log.Fatal(err)
			}

			tokenString = tokenString1

			msg = "کاربر در سیستم موجود است"
			if user.Password != in.Password {
				responseCodes = append(responseCodes, pb.CheckLoginResponse_WRONG_PASSWORD)
				msg = "رمز عبور اشتباه است"
				tokenString = ""
			}
			if user.Blocked {
				responseCodes = append(responseCodes, pb.CheckLoginResponse_MOBILE_BLOCKED)
				msg = "کاربر مسدود شده است"
				tokenString = ""
			}
			if user.Disabled {
				msg = "کاربر غیر فعال شده است"
				responseCodes = append(responseCodes, pb.CheckLoginResponse_DISABLED)
				tokenString = ""
			}
		} else {
			msg = "کاربر در سیستم موجود نیست"
			responseCodes = append(responseCodes, pb.CheckLoginResponse_UNREGISTERED)
			tokenString = ""
		}
	} else {
		responseCodes = append(responseCodes, pb.CheckLoginResponse_WRONG_MOBILE_NUMBER)
		msg = "شماره موبایل صحیح نیست"
		tokenString = ""
	}

	if len(responseCodes) == 0 {
		msg = "ورود موفق"
		responseCodes = append(responseCodes, pb.CheckLoginResponse_SUCCESS)
	}

	return &pb.CheckLoginResponse{
		Message:       msg,
		ResponseCodes: responseCodes,
		Token:         tokenString,
	}, nil
}

func (s *customerServer) VerifySMS(ctx context.Context, in *pb.VerifySmsRequest) (*pb.VerifySmsResponse, error) {
	log.Printf("customer verify sms request : %s", in.SmsCode)
	// Do database logic here....
	return &pb.VerifySmsResponse{
		Message:       "test",
		ResponseCodes: []pb.VerifySmsResponse_VerifyCustomerResponseCode{pb.VerifySmsResponse_SUCCESS},
		Token:         "token",
	}, nil
}

func (s *customerServer) Register(ctx context.Context, in *pb.RegisterCustomerRequest) (*pb.RegisterCustomerResponse, error) {
	log.Printf("customer register request : %s", in.MobileNumber)
	responseCodes := make([]pb.RegisterCustomerResponse_RegisterCustomerResponseCode, 0)
	msg := ""

	var user = models.User{
		PhoneNumber: in.MobileNumber,
		Password:    in.Password,
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
		PayCard: models.PayCard{
			Name:    "pay card",
			Balance: 10000,
		},
	}

	var checkUser models.User

	dbPool.Where("phone_number = ?", in.MobileNumber).First(&checkUser)

	if checkUser.PhoneNumber != in.MobileNumber {
		if len(in.Password) < 6 {
			responseCodes = append(responseCodes, pb.RegisterCustomerResponse_SHORT_PASSWORD)
			msg = "طول رمز عبور کوتاه است"
		} else {
			err := dbPool.Create(&user)
			if err.Error != nil {
				msg = "خطا در عملیات"
				responseCodes = append(responseCodes, pb.RegisterCustomerResponse_FAILED)
				log.Printf(err.Error.Error())
			} else {
				msg = "ثبت نام موفق"
				responseCodes = append(responseCodes, pb.RegisterCustomerResponse_SUCCESS)
			}
		}

	} else {
		msg = "کاربر در سیستم موجود است"
		responseCodes = append(responseCodes, pb.RegisterCustomerResponse_ALREADY_REGISTERED)
		if checkUser.Blocked {
			responseCodes = append(responseCodes, pb.RegisterCustomerResponse_MOBILE_BLOCKED)
			msg = "کاربر مسدود شده است"
		}
		if checkUser.Disabled {
			msg = "کاربر غیر فعال شده است"
			responseCodes = append(responseCodes, pb.RegisterCustomerResponse_DISABLED)
		}
	}

	return &pb.RegisterCustomerResponse{
		Message:       msg,
		ResponseCodes: responseCodes,
	}, nil
}

func (s *customerServer) AddCard(ctx context.Context, in *pb.AddCardRequest) (*pb.AddCardResponse, error) {
	log.Printf("customer add card request : %s", in.CardNumber)

	responseCodes := make([]pb.AddCardResponse_AddCardResponseCode, 0)
	var checkUser models.User
	var checkCard models.Card

	if len(in.Token) > 0 {

		dbPool.Where("phone_number = ?", in.MobileNumber).First(&checkUser)
		dbPool.Where("number = ?", in.CardNumber).First(&checkCard)

		if checkUser.PhoneNumber == in.MobileNumber {
			if checkUser.Blocked == false && checkUser.Disabled == false {
				if len(in.CardNumber) == 16 {

					var bank models.Bank
					switch in.CardNumber[0:6] {
					case "603799":
						bank.BankID = models.MelliID
						bank.Name = models.Melli
					case "589210":
						bank.BankID = models.SepahID
						bank.Name = models.Sepah
					case "627648":
						bank.BankID = models.TawseaaSaderatID
						bank.Name = models.TawseaaSaderat
					case "627961":
						bank.BankID = models.SanaatMaadanID
						bank.Name = models.SanaatMaadan
					case "603770":
						bank.BankID = models.KeshawarziID
						bank.Name = models.Keshawarzi
					case "628023":
						bank.BankID = models.MaskanID
						bank.Name = models.Maskan
					case "627760":
						bank.BankID = models.PostBankIranID
						bank.Name = models.PostBankIran
					case "502908":
						bank.BankID = models.TawseaaID
						bank.Name = models.Tawseaa
					case "627412":
						bank.BankID = models.EghtesadNovinID
						bank.Name = models.EghtesadNovin
					case "622106":
						bank.BankID = models.ParsianID
						bank.Name = models.Parsian
					case "502229":
						bank.BankID = models.PasargadID
						bank.Name = models.Pasargad
					case "627488":
						bank.BankID = models.KarAfarinID
						bank.Name = models.KarAfarin
					case "621986":
						bank.BankID = models.SamanID
						bank.Name = models.Saman
					case "639346":
						bank.BankID = models.SinaID
						bank.Name = models.Sina
					case "639607":
						bank.BankID = models.SarmayehID
						bank.Name = models.Sarmayeh
					case "636214":
						bank.BankID = models.TaatID
						bank.Name = models.Taat
					case "502806":
						bank.BankID = models.ShahrID
						bank.Name = models.Shahr
					case "502938":
						bank.BankID = models.DeyID
						bank.Name = models.Dey
					case "603769":
						bank.BankID = models.SaderatID
						bank.Name = models.Saderat
					case "610433":
						bank.BankID = models.MellatID
						bank.Name = models.Mellat
					case "627353":
						bank.BankID = models.TejaratID
						bank.Name = models.Tejarat
					case "585983":
						bank.BankID = models.TejaratID
						bank.Name = models.Tejarat
					case "589463":
						bank.BankID = models.RefahID
						bank.Name = models.Refah
					case "627381":
						bank.BankID = models.AnsarID
						bank.Name = models.Ansar
					case "639370":
						bank.BankID = models.MehreEghtesadID
						bank.Name = models.MehreEghtesad
					default:
						bank.BankID = -1
						bank.Name = ""
					}

					if bank.ID != -1 {
						card := models.Card{
							UserID:  checkUser.ID,
							Balance: 0,
							Bank:    bank,
							Number:  in.CardNumber,
							Year:    "99",
							Month:   "12",
							Cvv2:    "1234",
							Name:    "bank card",
						}
						if checkCard.Number != in.CardNumber {
							result := dbPool.Create(&card)
							if result.Error == nil {
								responseCodes = append(responseCodes, pb.AddCardResponse_SUCCESS)
							} else {
								responseCodes = append(responseCodes, pb.AddCardResponse_FAILED)
							}
						} else {
							responseCodes = append(responseCodes, pb.AddCardResponse_FAILED, pb.AddCardResponse_CARD_EXISTS)
						}
					} else {
						responseCodes = append(responseCodes, pb.AddCardResponse_INVALID_CARD)
					}
				} else {
					responseCodes = append(responseCodes, pb.AddCardResponse_INVALID_CARD)
				}

			} else {
				if checkUser.Blocked {
					responseCodes = append(responseCodes, pb.AddCardResponse_MOBILE_BLOCKED)
				}
				if checkUser.Disabled {
					responseCodes = append(responseCodes, pb.AddCardResponse_DISABLED)
				}
			}
		} else {
			responseCodes = append(responseCodes, pb.AddCardResponse_WRONG_USER)
		}
	} else {
		responseCodes = append(responseCodes, pb.AddCardResponse_WRONG_TOKEN)
	}

	return &pb.AddCardResponse{
		ResponseCodes: responseCodes,
	}, nil
}

func (s *customerServer) GetCard(ctx context.Context, in *pb.GetCardRequest) (*pb.GetCardResponse, error) {
	log.Printf("customer get card request : %s", in.CardNumber)

	responseCodes := make([]pb.GetCardResponse_GetCardResponseCode, 0)
	card := pb.Card{}
	var checkUser models.User
	var checkCard models.Card
	var checkPayCard models.PayCard

	if len(in.Token) > 0 {

		dbPool.Where("phone_number = ?", in.MobileNumber).First(&checkUser)
		if checkUser.PhoneNumber == in.MobileNumber {
			if checkUser.Blocked == false && checkUser.Disabled == false {
				switch in.Type {
				case pb.CardType_PayCard:
					dbPool.Where("user_id = ?", checkUser.ID).First(&checkPayCard)
					if checkPayCard.UserID != checkUser.ID {
						responseCodes = append(responseCodes, pb.GetCardResponse_INVALID_CARD)
					} else {
						if in.Passsowrd == checkUser.Password {
							card.Balance = checkPayCard.Balance
							card.Diabled = checkPayCard.Disabled
							card.Blocked = checkPayCard.Blocked
							card.CardName = checkPayCard.Name
							card.Type = pb.CardType_PayCard
							card.CardToken = ""
							responseCodes = append(responseCodes, pb.GetCardResponse_SUCCESS)
						} else {
							responseCodes = append(responseCodes, pb.GetCardResponse_WRONG_PASSWORD)
						}

					}
				case pb.CardType_BankCard:
					dbPool.Where("number = ?", in.CardNumber).First(&checkCard)
					if checkCard.Number != in.CardNumber {
						responseCodes = append(responseCodes, pb.GetCardResponse_INVALID_CARD)
					} else {
						if in.Passsowrd == "123456" {
							card.CardNumber = checkCard.Number
							card.Cvv2 = checkCard.Cvv2
							card.Month = checkCard.Month
							card.Year = checkCard.Year
							card.Balance = checkCard.Balance
							card.Diabled = checkCard.Disabled
							card.Blocked = checkCard.Blocked
							card.CardName = checkCard.Name
							card.Type = pb.CardType_BankCard
							card.CardToken = ""

							responseCodes = append(responseCodes, pb.GetCardResponse_SUCCESS)
						} else {
							responseCodes = append(responseCodes, pb.GetCardResponse_WRONG_PASSWORD)
						}

					}
				default:

				}

			} else {
				if checkUser.Blocked {
					responseCodes = append(responseCodes, pb.GetCardResponse_MOBILE_BLOCKED)
				}
				if checkUser.Disabled {
					responseCodes = append(responseCodes, pb.GetCardResponse_DISABLED)
				}
			}
		} else {
			responseCodes = append(responseCodes, pb.GetCardResponse_WRONG_USER)
		}
	} else {
		responseCodes = append(responseCodes, pb.GetCardResponse_WRONG_TOKEN)
	}

	return &pb.GetCardResponse{
		Card:          &card,
		ResponseCodes: responseCodes,
	}, nil
}

func (s *customerServer) GetCards(ctx context.Context, in *pb.GetCardsRequest) (*pb.GetCardsResponse, error) {
	log.Printf("customer get cards request : %s", in.MobileNumber)

	responseCodes := make([]pb.GetCardsResponse_GetCardsResponseCode, 0)
	cards := make([]*pb.Card, 0)
	var checkUser models.User
	var checkCards []models.Card
	var checkPayCard models.PayCard

	if len(in.Token) > 0 {

		dbPool.Where("phone_number = ?", in.MobileNumber).First(&checkUser)

		if checkUser.PhoneNumber == in.MobileNumber {
			if checkUser.Blocked == false && checkUser.Disabled == false {
				dbPool.Where("user_id = ?", checkUser.ID).First(&checkPayCard)
				if checkPayCard.UserID != checkUser.ID {
					responseCodes = append(responseCodes, pb.GetCardsResponse_SUCCESS)
				} else {
					// Set custom claims
					cardClaims := &models.CardClaim{
						PhoneNumber: checkUser.PhoneNumber,
						UserID:      checkUser.ID,
						CardID:      checkPayCard.ID,
						CardType:    0,
						StandardClaims: jwt.StandardClaims{
							Issuer:    "demo-project",
							Subject:   in.MobileNumber,
							ExpiresAt: time.Now().Add(time.Hour * 24).Unix(),
							NotBefore: time.Now().Unix(),
						},
					}

					// Create token with claims
					cardToken := jwt.NewWithClaims(jwt.SigningMethodHS256, cardClaims)

					// Generate encoded token and send it as response.
					tokenString, err := cardToken.SignedString([]byte("secret"))
					if err != nil {
						log.Fatal(err)
					}

					card := &pb.Card{
						Type:      pb.CardType_PayCard,
						CardName:  checkPayCard.Name,
						Blocked:   checkPayCard.Blocked,
						Diabled:   checkPayCard.Disabled,
						Balance:   checkPayCard.Balance,
						CardToken: tokenString,
					}
					cards = append(cards, card)
					responseCodes = append(responseCodes, pb.GetCardsResponse_SUCCESS)
				}

				dbPool.Where("user_id = ?", checkUser.ID).Find(&checkCards)

				var card *pb.Card
				for i := 0; i < len(checkCards); i++ {
					var checkBank models.Bank
					dbPool.Where("card_id = ?", checkCards[i].ID).Find(&checkBank)

					// Set custom claims
					cardClaims := &models.CardClaim{
						PhoneNumber: checkUser.PhoneNumber,
						UserID:      checkUser.ID,
						CardID:      checkCards[i].ID,
						CardType:    1,
						StandardClaims: jwt.StandardClaims{
							Issuer:    "demo-project",
							Subject:   in.MobileNumber,
							ExpiresAt: time.Now().Add(time.Hour * 24).Unix(),
							NotBefore: time.Now().Unix(),
						},
					}

					// Create token with claims
					cardToken := jwt.NewWithClaims(jwt.SigningMethodHS256, cardClaims)

					// Generate encoded token and send it as response.
					tokenString, err := cardToken.SignedString([]byte("secret"))
					if err != nil {
						log.Fatal(err)
					}

					card = &pb.Card{
						Type: pb.CardType_BankCard,
						Bank: &pb.BankType{
							BankID:   checkBank.BankID,
							BankName: checkBank.Name,
						},
						CardName:   checkCards[i].Name,
						Blocked:    checkCards[i].Blocked,
						Diabled:    checkCards[i].Disabled,
						Balance:    checkCards[i].Balance,
						Cvv2:       checkCards[i].Cvv2,
						Month:      checkCards[i].Month,
						Year:       checkCards[i].Year,
						CardNumber: checkCards[i].Number,
						CardToken:  tokenString,
					}
					cards = append(cards, card)

				}

				responseCodes = append(responseCodes, pb.GetCardsResponse_SUCCESS)
			} else {
				if checkUser.Blocked {
					responseCodes = append(responseCodes, pb.GetCardsResponse_MOBILE_BLOCKED)
				}
				if checkUser.Disabled {
					responseCodes = append(responseCodes, pb.GetCardsResponse_DISABLED)
				}
			}
		} else {
			responseCodes = append(responseCodes, pb.GetCardsResponse_WRONG_USER)
		}
	} else {
		responseCodes = append(responseCodes, pb.GetCardsResponse_WRONG_TOKEN)
	}

	return &pb.GetCardsResponse{
		Cards:         cards,
		ResponseCodes: responseCodes,
	}, nil
}

func (s *customerServer) CheckCardPassword(ctx context.Context, in *pb.CheckCardPasswordRequest) (*pb.CheckCardPasswordResponse, error) {
	log.Printf("customer check password request : %s", in.MobileNumber)

	responseCodes := make([]pb.CheckCardPasswordResponse_CheckCardPasswordResponseCode, 0)
	var checkUser models.User
	var checkCard models.Card
	var checkPayCard models.PayCard

	if len(in.Token) > 0 {

		dbPool.Where("phone_number = ?", in.MobileNumber).First(&checkUser)

		if checkUser.PhoneNumber == in.MobileNumber {
			if checkUser.Blocked == false && checkUser.Disabled == false {
				switch in.Type {
				case pb.CardType_PayCard:
					dbPool.Where("user_id = ?", checkUser.ID).First(&checkPayCard)
					if checkPayCard.UserID != checkUser.ID {
						responseCodes = append(responseCodes, pb.CheckCardPasswordResponse_WRONG_USER)
					} else {
						if checkUser.Password == in.Password {
							responseCodes = append(responseCodes, pb.CheckCardPasswordResponse_SUCCESS)
						} else {
							responseCodes = append(responseCodes, pb.CheckCardPasswordResponse_WRONG_PASSWORD)
						}
					}
				case pb.CardType_BankCard:
					dbPool.Where("user_id = ?", checkUser.ID).First(&checkCard)

					//TODO change this later
					if in.Password == "123456" {
						responseCodes = append(responseCodes, pb.CheckCardPasswordResponse_SUCCESS)
					} else {
						responseCodes = append(responseCodes, pb.CheckCardPasswordResponse_WRONG_PASSWORD)
					}
				default:

				}

			} else {
				if checkUser.Blocked {
					responseCodes = append(responseCodes, pb.CheckCardPasswordResponse_MOBILE_BLOCKED)
				}
				if checkUser.Disabled {
					responseCodes = append(responseCodes, pb.CheckCardPasswordResponse_DISABLED)
				}
			}
		} else {
			responseCodes = append(responseCodes, pb.CheckCardPasswordResponse_WRONG_USER)
		}
	} else {
		responseCodes = append(responseCodes, pb.CheckCardPasswordResponse_WRONG_TOKEN)
	}

	return &pb.CheckCardPasswordResponse{
		ResponseCodes: responseCodes,
	}, nil
}

func (s *customerServer) RemoveCard(ctx context.Context, in *pb.RemoveCardRequest) (*pb.RemoveCardResponse, error) {
	log.Printf("customer remove card request : %s", in.CardNumber)

	responseCodes := make([]pb.RemoveCardResponse_RemoveCardResponseCode, 0)
	var checkUser models.User
	var checkCard models.Card

	if len(in.Token) > 0 {

		dbPool.Where("phone_number = ?", in.MobileNumber).First(&checkUser)

		if checkUser.PhoneNumber == in.MobileNumber {
			if checkUser.Blocked == false && checkUser.Disabled == false {

				dbPool.Where("number = ?", in.CardNumber).First(&checkCard)
				if checkCard.Number != in.CardNumber {
					responseCodes = append(responseCodes, pb.RemoveCardResponse_INVALID_CARD)
				} else {
					if result := dbPool.Where("user_id = ? and number = ?", checkUser.ID, in.CardNumber).Delete(&checkCard); result.Error == nil {
						responseCodes = append(responseCodes, pb.RemoveCardResponse_SUCCESS)
					} else {
						responseCodes = append(responseCodes, pb.RemoveCardResponse_FAILED)
					}
				}
			} else {
				if checkUser.Blocked {
					responseCodes = append(responseCodes, pb.RemoveCardResponse_MOBILE_BLOCKED)
				}
				if checkUser.Disabled {
					responseCodes = append(responseCodes, pb.RemoveCardResponse_DISABLED)
				}
			}
		} else {
			responseCodes = append(responseCodes, pb.RemoveCardResponse_WRONG_USER)
		}
	} else {
		responseCodes = append(responseCodes, pb.RemoveCardResponse_WRONG_TOKEN)
	}

	return &pb.RemoveCardResponse{
		ResponseCodes: responseCodes,
	}, nil
}

func (s *customerServer) Transaction(ctx context.Context, in *pb.TransactionRequest) (*pb.TransactionResponse, error) {
	log.Printf("customer transaction request : %s", in.Token)
	responseCodes := make([]pb.TransactionResponse_TransactionResponseCode, 0)

	argSlice := strings.Split(in.Token, ":")
	//0: src card token
	//1: price
	//2: dst app token

	if len(argSlice) == 3 {

		cardToken, err := jwt.ParseWithClaims(argSlice[0], &models.CardClaim{}, func(token *jwt.Token) (interface{}, error) {
			return []byte("secret"), nil
		})

		appToken, err := jwt.ParseWithClaims(argSlice[2], &models.AppClaim{}, func(token *jwt.Token) (interface{}, error) {
			return []byte("secret"), nil
		})

		at(time.Now(), func() {
			if cardClaim, ok := cardToken.Claims.(*models.CardClaim); ok && cardToken.Valid {
				fmt.Printf("%v %v", cardClaim.UserID, cardClaim.StandardClaims.ExpiresAt)

				at(time.Now(), func() {
					if appClaims, ok := appToken.Claims.(*models.AppClaim); ok && cardToken.Valid {
						fmt.Printf("%v %v", appClaims.UserID, appClaims.StandardClaims.ExpiresAt)

						var checkUser models.User
						dbPool.Where("id = ?", cardClaim.UserID).First(&checkUser)
						if checkUser.ID == cardClaim.UserID {
							switch cardClaim.CardType {
							case 0:
								//PayCard
								var checkCard models.PayCard
								dbPool.Where("user_id =? and id = ?", cardClaim.UserID, cardClaim.CardID).First(&checkCard)
								if checkCard.UserID == cardClaim.UserID {
									amount, err := strconv.ParseInt(argSlice[1], 10, 64)
									if err != nil {
										responseCodes = append(responseCodes, pb.TransactionResponse_FAILED)
									} else {
										if checkCard.Balance >= amount {
											var checkDstPayCard models.PayCard
											dbPool.Where("user_id =?", appClaims.UserID).First(&checkDstPayCard)
											if checkDstPayCard.UserID == appClaims.UserID {
												err := dbPool.Create(&models.Transaction{
													Amount:          amount,
													SourceUser:      cardClaim.UserID,
													DestinationUser: appClaims.UserID,
													TransactionID:   time.Now().String() + ":" + strconv.FormatInt(amount, 10) + ":" + strconv.FormatInt(rand.Int63n(10000000), 10),
												})
												if err.Error != nil {
													responseCodes = append(responseCodes, pb.TransactionResponse_FAILED)
													log.Printf(err.Error.Error())
												} else {
													checkCard.Balance -= amount
													checkDstPayCard.Balance += amount
													err1 := dbPool.Save(&checkCard)
													if err1.Error != nil {
														responseCodes = append(responseCodes, pb.TransactionResponse_FAILED)
														log.Printf(err1.Error.Error())
													} else {
														err2 := dbPool.Save(&checkDstPayCard)
														if err2.Error != nil {
															responseCodes = append(responseCodes, pb.TransactionResponse_FAILED)
															log.Printf(err2.Error.Error())
														} else {
															responseCodes = append(responseCodes, pb.TransactionResponse_SUCCESS)
														}
													}
												}

											} else {
												responseCodes = append(responseCodes, pb.TransactionResponse_WRONG_USER)
											}
										} else {
											responseCodes = append(responseCodes, pb.TransactionResponse_INSUFFICIENT_BALANCE)
										}
									}
								} else {
									responseCodes = append(responseCodes, pb.TransactionResponse_WRONG_USER)
								}
							case 1:
								//BankCard
								var checkCard models.Card
								dbPool.Where("user_id =? and id = ?", cardClaim.UserID, cardClaim.CardID).First(&checkCard)
								if checkCard.UserID == cardClaim.UserID {
									amount, err := strconv.ParseInt(argSlice[1], 10, 64)
									if err != nil {
										responseCodes = append(responseCodes, pb.TransactionResponse_FAILED)
									} else {
										if checkCard.Balance >= amount {
											var checkDstPayCard models.PayCard
											dbPool.Where("user_id =?", appClaims.UserID).First(&checkDstPayCard)
											if checkDstPayCard.UserID == appClaims.UserID {
												err := dbPool.Create(&models.Transaction{
													Amount:           amount,
													SourceUser:       cardClaim.UserID,
													SourceCardNumber: checkCard.Number,
													DestinationUser:  appClaims.UserID,
													TransactionID:    time.Now().String() + ":" + strconv.FormatInt(amount, 10) + ":" + strconv.FormatInt(rand.Int63n(10000000), 10),
												})
												if err.Error != nil {
													responseCodes = append(responseCodes, pb.TransactionResponse_FAILED)
													log.Printf(err.Error.Error())
												} else {
													checkCard.Balance -= amount
													checkDstPayCard.Balance += amount
													err1 := dbPool.Save(&checkCard)
													if err1.Error != nil {
														responseCodes = append(responseCodes, pb.TransactionResponse_FAILED)
														log.Printf(err1.Error.Error())
													} else {
														err2 := dbPool.Save(&checkDstPayCard)
														if err2.Error != nil {
															responseCodes = append(responseCodes, pb.TransactionResponse_FAILED)
															log.Printf(err2.Error.Error())
														} else {
															responseCodes = append(responseCodes, pb.TransactionResponse_SUCCESS)
														}
													}
												}

											} else {
												responseCodes = append(responseCodes, pb.TransactionResponse_WRONG_USER)
											}
										} else {
											responseCodes = append(responseCodes, pb.TransactionResponse_INSUFFICIENT_BALANCE)
										}
									}
								} else {
									responseCodes = append(responseCodes, pb.TransactionResponse_WRONG_USER)
								}
							default:
								responseCodes = append(responseCodes, pb.TransactionResponse_WRONG_TOKEN)
							}

						} else {
							responseCodes = append(responseCodes, pb.TransactionResponse_WRONG_USER)
						}

					} else {
						fmt.Println(err)
						responseCodes = append(responseCodes, pb.TransactionResponse_WRONG_TOKEN)
					}
				})
			} else {
				fmt.Println(err)
				responseCodes = append(responseCodes, pb.TransactionResponse_WRONG_TOKEN)
			}
		})

	} else {
		responseCodes = append(responseCodes, pb.TransactionResponse_WRONG_TOKEN)
	}

	return &pb.TransactionResponse{
		ResponseCodes: responseCodes,
	}, nil
}

func at(t time.Time, f func()) {
	jwt.TimeFunc = func() time.Time {
		return t
	}
	f()
	jwt.TimeFunc = time.Now
}

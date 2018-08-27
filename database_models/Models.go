package database_models

import (
	"github.com/dgrijalva/jwt-go"
	"time"
)

const (
	Melli          = "بانک ملی"
	Sepah          = "بانک سپه"
	TawseaaSaderat = "بانک توسعه صادرات"
	SanaatMaadan   = "بانک صنعت و معدن"
	Keshawarzi     = "بانک کشاورزی"
	Maskan         = "بانک مسکن"
	PostBankIran   = "پست بانک ایران"
	Tawseaa        = "بانک توسعه تعاون"
	EghtesadNovin  = "بانک اقتصاد نوین"
	Parsian        = "بانک پارسیان"
	Pasargad       = "بانک پاسارگاد"
	KarAfarin      = "بانک کارآفرین"
	Saman          = "بانک سامان"
	Sina           = "بانک سینا"
	Sarmayeh       = "بانک سرمایه"
	Taat           = "بانک تات"
	Shahr          = "بانک شهر"
	Dey            = "بانک دی"
	Saderat        = "بانک صادرات"
	Mellat         = "بانک ملت"
	Tejarat        = "بانک تجارت"
	Refah          = "بانک رفاه"
	Ansar          = "بانک انصار"
	MehreEghtesad  = "بانک مهر اقتصاد"
)

const (
	MelliID          = 0
	SepahID          = 1
	TawseaaSaderatID = 2
	SanaatMaadanID   = 3
	KeshawarziID     = 4
	MaskanID         = 5
	PostBankIranID   = 6
	TawseaaID        = 7
	EghtesadNovinID  = 8
	ParsianID        = 9
	PasargadID       = 10
	KarAfarinID      = 11
	SamanID          = 12
	SinaID           = 13
	SarmayehID       = 14
	TaatID           = 15
	ShahrID          = 16
	DeyID            = 17
	SaderatID        = 18
	MellatID         = 19
	TejaratID        = 20
	RefahID          = 21
	AnsarID          = 22
	MehreEghtesadID  = 23
)

type User struct {
	ID                int64  `gorm:"AUTO_INCREMENT;primary_key"`
	FirstName         string `gorm:"size:255;default:''"` // Default size for string is 255, reset it with this tag
	LastName          string `gorm:"size:255;default:''"` // Default size for string is 255, reset it with this tag
	PhoneNumber       string `gorm:"type:varchar(20);unique_index;default:''"`
	Password          string `gorm:"size:255;default:'123456'"`
	PasswordRetryLeft int32  `gorm:"default:'10'"`
	Cards             []Card `gorm:"foreignkey:UserID;"`
	//Email              string  `gorm:"default:'';unique_index;"`
	CreatedAt          time.Time
	UpdatedAt          time.Time
	Disabled           bool    `gorm:"default:'0'"`
	Blocked            bool    `gorm:"default:'0'"`
	IsMerchant         bool    `gorm:"default:'0'"`
	MerchantID         string  `gorm:"default:'';"`
	IsMerchantDisabled bool    `gorm:"default:'0'"`
	PayCard            PayCard `gorm:"foreignkey:UserID;"`
	IsMerchantBlocked  bool    `gorm:"default:'0'"`
}

type Card struct {
	ID       int64 `gorm:"AUTO_INCREMENT;primary_key"`
	UserID   int64
	Name     string `gorm:"size:255;default:'';not null;"`
	Bank     Bank   `gorm:"foreignkey:CardID;"`
	Number   string `gorm:"default:'0'"`
	Balance  int64  `gorm:"default:'0'"`
	Year     string `gorm:"default:'96'"`
	Month    string `gorm:"default:'1'"`
	Cvv2     string `gorm:"default:''"`
	Disabled bool   `gorm:"default:'0'"`
	Blocked  bool   `gorm:"default:'0'"`
}

type PayCard struct {
	ID       int64 `gorm:"AUTO_INCREMENT;primary_key"`
	UserID   int64
	Balance  int64  `gorm:"default:'0'"`
	Name     string `gorm:"size:255;default:'';not null;"`
	Disabled bool   `gorm:"default:'0'"`
	Blocked  bool   `gorm:"default:'0'"`
}

type Bank struct {
	ID     int64 `gorm:"AUTO_INCREMENT;primary_key"`
	CardID int64
	BankID int32  `gorm:"size:255;default:'-1';not null;"`
	Name   string `gorm:"size:255;default:'';not null;"`
}

type Transaction struct {
	ID                    int64  `gorm:"AUTO_INCREMENT;primary_key"`
	TransactionID         string `gorm:"size:255;default:'';unique_index;"`
	SourceCardNumber      string `gorm:"size:255;default:'';"`
	DestinationCardNumber string `gorm:"size:255;default:'';"`
	SourceUser            int64  `gorm:"not null;"`
	DestinationUser       int64
	Amount                int64 `gorm:"default:'0'"`
}

////////////////////////////////////////////////
type SmsCodeClaims struct {
	PhoneNumber string `json:"phone_number"`
	SmsCode     int32  `json:"sms_code"`
	jwt.StandardClaims
}

type AppClaim struct {
	PhoneNumber string `json:"phone_number"`
	UserID      int64  `json:"user_id"`
	jwt.StandardClaims
}

type CardClaim struct {
	PhoneNumber string `json:"phone_number"`
	UserID      int64  `json:"user_id"`
	CardID      int64  `json:"card_id"`
	CardType    int32  `json:"card_type"`
	jwt.StandardClaims
}

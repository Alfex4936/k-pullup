package config

import (
	"encoding/base64"
	"os"
	"strconv"
	"time"

	"golang.org/x/oauth2"
	"golang.org/x/oauth2/github"
	"golang.org/x/oauth2/google"
	"golang.org/x/oauth2/kakao"
)

type AppConfig struct {
	TokenExpirationTime time.Duration
	AwsRegion           string
	S3BucketName        string
	LoginTokenCookie    string
	IsProduction        string
	IsWaterURL          string
	IsWaterKEY          string
	CkURL               string // to fetch new ones from external
	NaverEmailVerifyURL string
	ClientURL           string
	TestValue           string
	TokenLength         int
	TokenConcurrentMax  int
}

func NewAppConfig() *AppConfig {
	expiration, err := strconv.Atoi(os.Getenv("TOKEN_EXPIRATION_INTERVAL"))
	if err != nil {
		expiration = 24 // default to 24 hour if not set or error occurs
	}

	return &AppConfig{
		AwsRegion:           os.Getenv("AWS_REGION"),
		S3BucketName:        os.Getenv("AWS_BUCKET_NAME"),
		LoginTokenCookie:    os.Getenv("TOKEN_COOKIE"),
		TokenExpirationTime: time.Duration(expiration) * time.Hour,
		IsProduction:        os.Getenv("DEPLOYMENT"),
		IsWaterURL:          os.Getenv("IS_WATER_API"),
		IsWaterKEY:          os.Getenv("IS_WATER_API_KEY"),
		CkURL:               os.Getenv("CK_URL"),
		NaverEmailVerifyURL: os.Getenv("NAVER_EMAIL_VERIFY_URL"),
		ClientURL:           os.Getenv("CLIENT_ADDR"),
		TokenLength:         32,
		TokenConcurrentMax:  3, // 3 tokens per user,
		TestValue:           os.Getenv("TEST_VALUE"),
	}
}

type KakaoConfig struct {
	KakaoAK           string
	KakaoStaticMap    string
	KakaoGeoCode      string
	KakaoCoord2Addr   string
	KakaoCoord2Region string
	KakaoWeather      string
	KakaoAddressInfo  string
	KakaoRoadViewAPI  string
}

func NewKakaoConfig() *KakaoConfig {
	return &KakaoConfig{
		KakaoAK:           os.Getenv("KAKAO_AK"),
		KakaoStaticMap:    os.Getenv("KAKAO_STATIC_MAP"),
		KakaoGeoCode:      "https://dapi.kakao.com/v2/local/geo/address.json",
		KakaoCoord2Addr:   "https://dapi.kakao.com/v2/local/geo/coord2address.json",
		KakaoCoord2Region: "https://dapi.kakao.com/v2/local/geo/coord2regioncode.json",
		KakaoWeather:      "https://map.kakao.com/api/dapi/point/weather?inputCoordSystem=WCONGNAMUL&outputCoordSystem=WCONGNAMUL&version=2&service=map.daum.net",
		KakaoAddressInfo:  "https://map.kakao.com/etc/areaAddressInfo.json?output=JSON&inputCoordSystem=WCONGNAMUL&outputCoordSystem=WCONGNAMUL",
		KakaoRoadViewAPI:  os.Getenv("KAKAO_ROADVIEW_API"),
	}
}

type RedisConfig struct {
	AllMarkersKey         string
	UserProfileKey        string
	UserFavKey            string
	KakaoRecentMarkersKey string
	KakaoSearchMarkersKey string
}

func NewRedisConfig() *RedisConfig {
	return &RedisConfig{
		AllMarkersKey:         "all_markers",
		UserProfileKey:        "user_profile",
		UserFavKey:            "user_fav",
		KakaoRecentMarkersKey: "kakaobot:recent-markers",
		KakaoSearchMarkersKey: "kakaobot:search-markers:",
	}
}

type ZincSearchConfig struct {
	ZincAPI      string
	ZincUser     string
	ZincPassword string
}

func NewZincSearchConfig() *ZincSearchConfig {
	return &ZincSearchConfig{
		ZincAPI:      os.Getenv("ZINCSEARCH_URL"),
		ZincUser:     os.Getenv("ZINCSEARCH_USER"),
		ZincPassword: os.Getenv("ZINCSEARCH_PASSWORD"),
	}
}

type S3Config struct {
	ImageCacheExpirationTime time.Duration
	AwsRegion                string
	S3BucketName             string
}

func NewS3Config() *S3Config {
	return &S3Config{
		AwsRegion:                os.Getenv("AWS_REGION"),
		S3BucketName:             os.Getenv("AWS_BUCKET_NAME"),
		ImageCacheExpirationTime: 180 * time.Minute,
	}
}

type SmtpConfig struct {
	SmtpServer          string
	SmtpPort            string
	SmtpUsername        string
	SmtpPassword        string
	FrontendResetRouter string
}

func NewSmtpConfig() *SmtpConfig {
	return &SmtpConfig{
		SmtpServer:          os.Getenv("SMTP_SERVER"),
		SmtpPort:            os.Getenv("SMTP_PORT"),
		SmtpUsername:        os.Getenv("SMTP_USERNAME"),
		SmtpPassword:        os.Getenv("SMTP_PASSWORD"),
		FrontendResetRouter: os.Getenv("FRONTEND_RESET_ROUTER"),
	}
}

type TossPayConfig struct {
	SecretKey  string
	ConfirmAPI string
	PaymentAPI string
}

func NewTossPayConfig() *TossPayConfig {
	return &TossPayConfig{
		SecretKey:  "Basic " + base64.StdEncoding.EncodeToString([]byte(os.Getenv("TOSS_SECRET_KEY_TEST")+":")),
		ConfirmAPI: "https://api.tosspayments.com/v1/payments/confirm",
		PaymentAPI: "https://api.tosspayments.com/v1/payments/",
	}
}

type OAuthConfig struct {
	FrontendURL       string
	MobileRedirectURL string
	GoogleOAuth       *oauth2.Config
	KakaoOAuth        *oauth2.Config
	NaverOAuth        *oauth2.Config
	GitHubOAuth       *oauth2.Config
}

func NewOAuthConfig() *OAuthConfig {
	return &OAuthConfig{
		GoogleOAuth: &oauth2.Config{
			// RedirectURL: "http://localhost:8080/api/v1/auth/google",
			RedirectURL:  "https://api.k-pullup.com/api/v1/auth/google",
			ClientID:     os.Getenv("OAUTH_GOOGLE_CLIENT_ID"),
			ClientSecret: os.Getenv("OAUTH_GOOGLE_CLIENT_SECRET"),
			Scopes:       []string{"https://www.googleapis.com/auth/userinfo.email", "https://www.googleapis.com/auth/userinfo.profile"},
			Endpoint:     google.Endpoint,
		},

		KakaoOAuth: &oauth2.Config{
			// RedirectURL: "http://localhost:8080/api/v1/auth/kakao",
			RedirectURL:  "https://api.k-pullup.com/api/v1/auth/kakao",
			ClientID:     os.Getenv("OAUTH_KAKAO_CLIENT_ID"),
			ClientSecret: os.Getenv("OAUTH_KAKAO_CLIENT_SECRET"),
			Scopes:       []string{"profile_nickname,account_email,profile_image"},
			Endpoint:     kakao.Endpoint,
		},

		NaverOAuth: &oauth2.Config{
			// RedirectURL:  "http://localhost:8080/api/v1/auth/naver",
			RedirectURL:  "https://api.k-pullup.com/api/v1/auth/naver",
			ClientID:     os.Getenv("OAUTH_NAVER_CLIENT_ID"),
			ClientSecret: os.Getenv("OAUTH_NAVER_CLIENT_SECRET"),
			Scopes:       []string{"name,email,profile_image"},
			Endpoint: oauth2.Endpoint{
				AuthURL:  "https://nid.naver.com/oauth2.0/authorize",
				TokenURL: "https://nid.naver.com/oauth2.0/token",
			},
		},
		GitHubOAuth: &oauth2.Config{
			RedirectURL:  "https://api.k-pullup.com/api/v1/auth/github",
			ClientID:     os.Getenv("OAUTH_GITHUB_CLIENT_ID"),
			ClientSecret: os.Getenv("OAUTH_GITHUB_CLIENT_SECRET"),
			Scopes:       []string{"user:email"},
			Endpoint:     github.Endpoint,
		},

		FrontendURL:       "https://k-pullup.com",
		MobileRedirectURL: "kpullup://login",
	}
}

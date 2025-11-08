package dto

import "fmt"

type OAuthGoogleUser struct {
	ID           string `json:"id"`
	Email        string `json:"email"`
	Name         string `json:"name"`
	ProfileImage string `json:"picture,omitempty"`
}

// KakaoProfile represents the profile information in the kakao_account object
type KakaoProfile struct {
	Nickname       string `json:"nickname"`
	ThumbnailImage string `json:"thumbnail_image_url,omitempty"`
	ProfileImage   string `json:"profile_image_url,omitempty"`
}

// KakaoAccount represents the kakao_account object
type KakaoAccount struct {
	Email   string       `json:"email"`
	Profile KakaoProfile `json:"profile"`
}

// OAuthKakaoUser represents the user information returned by Kakao
type OAuthKakaoUser struct {
	ID           int64        `json:"id"`
	KakaoAccount KakaoAccount `json:"kakao_account"`
	Properties   struct {
		Nickname string `json:"nickname"`
	} `json:"properties"`
}

type OAuthNaverUser struct {
	Response struct {
		ID           string `json:"id"`
		Email        string `json:"email"`
		Nickname     string `json:"nickname"`
		ProfileImage string `json:"profile_image,omitempty"`
	} `json:"response"`
}

type OAuthGitHubUser struct {
	ID    int    `json:"id"`
	Email string `json:"email"`
	Name  string `json:"name"`
}

// OAuthUser is an interface for different OAuth provider users
type OAuthUser interface {
	GetID() string
	GetEmail() string
	GetName() string
}

// Implement OAuthUser interface for OAuthGoogleUser
func (u *OAuthGoogleUser) GetID() string {
	return u.ID
}

func (u *OAuthGoogleUser) GetEmail() string {
	return u.Email
}

func (u *OAuthGoogleUser) GetName() string {
	return u.Name
}

// Implement OAuthUser interface for OAuthKakaoUser
func (u *OAuthKakaoUser) GetID() string {
	return fmt.Sprintf("%d", u.ID)
}

func (u *OAuthKakaoUser) GetEmail() string {
	return u.KakaoAccount.Email
}

func (u *OAuthKakaoUser) GetName() string {
	if u.KakaoAccount.Profile.Nickname != "" {
		return u.KakaoAccount.Profile.Nickname
	}
	return u.Properties.Nickname
}

// Implement OAuthUser interface for OAuthNaverUser
func (u *OAuthNaverUser) GetID() string {
	return u.Response.ID
}

func (u *OAuthNaverUser) GetEmail() string {
	return u.Response.Email
}

func (u *OAuthNaverUser) GetName() string {
	return u.Response.Nickname
}

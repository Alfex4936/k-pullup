package handler

import (
	"context"
	"database/sql"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/Alfex4936/chulbong-kr/dto"
	"github.com/Alfex4936/chulbong-kr/middleware"
	"github.com/Alfex4936/chulbong-kr/service"
	"github.com/Alfex4936/chulbong-kr/util"
	sonic "github.com/bytedance/sonic"
	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/recover"
	"github.com/prometheus/client_golang/prometheus"
	"go.uber.org/zap"
	"golang.org/x/oauth2"
)

const (
	GOOGLE_USER_INFO_URL = "https://www.googleapis.com/oauth2/v2/userinfo"
	KAKAO_USER_INFO_URL  = "https://kapi.kakao.com/v2/user/me"
	NAVER_USER_INFO_URL  = "https://openapi.naver.com/v1/nid/me"
)

type SimpleErrorResponse dto.SimpleErrorResponse

type AuthHandler struct {
	AuthService  *service.AuthService
	UserService  *service.UserService
	TokenService *service.TokenService
	SmtpService  *service.SmtpService

	TokenUtil    *util.TokenUtil
	Logger       *zap.Logger
	LoginCounter prometheus.Counter
}

// NewAuthHandler creates a new AuthHandler with dependencies injected
func NewAuthHandler(
	auth *service.AuthService,
	user *service.UserService,
	token *service.TokenService,
	smtp *service.SmtpService,
	tutil *util.TokenUtil,
	logger *zap.Logger,
	lc prometheus.Counter,
) *AuthHandler {
	return &AuthHandler{
		AuthService:  auth,
		UserService:  user,
		TokenService: token,
		SmtpService:  smtp,
		TokenUtil:    tutil,
		Logger:       logger,
		LoginCounter: lc,
	}
}

// RegisterAuthRoutes sets up the routes for auth handling within the application.
func RegisterAuthRoutes(api fiber.Router, handler *AuthHandler, authMiddleaware *middleware.AuthMiddleware) {
	authGroup := api.Group("/auth")

	authGroup.Use(recover.New(recover.Config{
		EnableStackTrace: true,
		StackTraceHandler: func(c *fiber.Ctx, e any) {
			handler.Logger.Error("Panic recovered in auth API",
				zap.Any("error", e),
				zap.String("url", c.Path()),
				zap.String("method", c.Method()),
			)
		},
	}))

	{
		// OAuth2
		authGroup.Get("/google", handler.HandleOAuthProvider("google"))
		authGroup.Get("/naver", handler.HandleOAuthProvider("naver"))
		authGroup.Get("/kakao", handler.HandleOAuthProvider("kakao"))
		// authGroup.Get("/github", handler.HandleOAuthProvider("github"))

		authGroup.Post("/signup", handler.HandleSignUp)
		authGroup.Post("/login", handler.HandleLogin)
		authGroup.Post("/logout", authMiddleaware.Verify, handler.HandleLogout)
		authGroup.Post("/verify-email/send", handler.HandleSendVerificationEmail)
		authGroup.Post("/verify-email/confirm", handler.HandleValidateToken)

		// Finding password
		authGroup.Post("/request-password-reset", handler.HandleRequestResetPassword)
		authGroup.Post("/reset-password", handler.HandleResetPassword)
	}

}

// func (h *AuthHandler) HandleGoogleLogin(c *fiber.Ctx) error {
// 	url := h.AuthService.OAuthConfig.GoogleOAuth.AuthCodeURL("state")
// 	c.Status(fiber.StatusSeeOther)
// 	return c.Redirect(url)
// }

// func (h *AuthHandler) HandleKakaoLogin(c *fiber.Ctx) error {
// 	url := h.AuthService.OAuthConfig.KakaoOAuth.AuthCodeURL("state")
// 	c.Status(fiber.StatusSeeOther)
// 	return c.Redirect(url)
// }

// SignUp User godoc
//
//	@Summary		Sign up a new user [normal]
//	@Description	This endpoint is responsible for registering a new user in the system.
//	@Description	It checks the verification status of the user's email before proceeding.
//	@Description	If the email is not verified, it returns an error.
//	@Description	On successful creation, it returns the user's information.
//	@ID				sign-up-user
//	@Tags			auth
//	@Accept			json
//	@Produce		json
//	@Param			signUpRequest	body	dto.SignUpRequest	true	"SignUp Request"
//	@Security
//	@Success		201	{object}	model.User		"User registered successfully"
//	@Failure		400	{object}	map[string]interface{}	"Cannot parse JSON, wrong sign up form."
//	@Failure		400	{object}	map[string]interface{}	"Email not verified"
//	@Failure		409	{object}	map[string]interface{}	"Email already registered"
//	@Failure		500	{object}	map[string]interface{}	"An error occurred while creating the user"
//	@Router			/api/v1/auth/signup [post]
func (h *AuthHandler) HandleSignUp(c *fiber.Ctx) error {
	var signUpReq dto.SignUpRequest
	if err := c.BodyParser(&signUpReq); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(SimpleErrorResponse{Error: "Cannot parse JSON, wrong sign up form."})
	}

	// Check if the token is verified before proceeding
	verified, err := h.TokenService.IsTokenVerified(signUpReq.Email)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(SimpleErrorResponse{Error: "Failed to check verification status"})
	}
	if !verified {
		return c.Status(fiber.StatusBadRequest).JSON(SimpleErrorResponse{Error: "Email not verified"})
	}

	signUpReq.Provider = "website"

	user, err := h.AuthService.SaveUser(&signUpReq)
	if err != nil {
		// Handle the duplicate email error
		if strings.Contains(err.Error(), "already registered") {
			return c.Status(fiber.StatusConflict).JSON(SimpleErrorResponse{Error: "Duplicate email address"})
		}
		// For other errors, return a generic error message
		return c.Status(fiber.StatusInternalServerError).JSON(SimpleErrorResponse{Error: "An error occurred while creating the user"})
	}

	return c.Status(fiber.StatusCreated).JSON(user)
}

// Login User godoc
//
// @Summary		Log in a user
// @Description	This endpoint is responsible for authenticating a user in the system.
// @Description	It validates the user's login credentials (email and password).
// @Description	If the credentials are invalid, it returns an error.
// @Description	On successful authentication, it returns the user's information along with a token.
// @Description	The token is also set in a secure cookie for client-side storage.
// @ID			login-user
// @Tags		auth
// @Accept		json
// @Produce	json
// @Security
// @Param		loginRequest	body	dto.LoginRequest	true	"Login Request"
// @Success	200	{object}	dto.LoginResponse	"User logged in successfully, includes user info and token"
// @Failure	400	{object}	map[string]interface{}	"Cannot parse JSON, wrong login form."
// @Failure	401	{object}	map[string]interface{}	"Invalid email or password"
// @Failure	500	{object}	map[string]interface{}	"Failed to generate token"
// @Router		/api/v1/auth/login [post]
func (h *AuthHandler) HandleLogin(c *fiber.Ctx) error {
	var request dto.LoginRequest
	// if err := c.BodyParser(&request); err != nil {
	if err := util.JsonBodyParserFast(c, &request); err != nil {
		h.Logger.Error("Failed to parse login request body", zap.Error(err))
		return c.Status(fiber.StatusBadRequest).JSON(SimpleErrorResponse{Error: "Invalid login request"})
	}

	user, err := h.AuthService.Login(request.Email, request.Password)
	if err != nil {
		h.Logger.Warn("Login failed", zap.Error(err))
		return c.Status(fiber.StatusUnauthorized).JSON(SimpleErrorResponse{Error: "Invalid email or password"})
	}

	token, err := h.TokenService.GenerateAndSaveToken(user.UserID)
	if err != nil {
		h.Logger.Error("Failed to generate token", zap.Error(err))
		return c.Status(fiber.StatusInternalServerError).JSON(SimpleErrorResponse{Error: "Failed to generate token"})
	}

	// Create a response object that includes both the user and the token
	response := dto.LoginResponse{
		User:  user,
		Token: token,
	}

	// Setting the token in a secure cookie
	cookie := h.TokenUtil.GenerateLoginCookie(token)
	c.Cookie(&cookie)

	h.LoginCounter.Inc() // Increment the login counter.
	h.Logger.Info("User logged in successfully", zap.Int("userID", user.UserID))

	return c.JSON(response)
}

func (h *AuthHandler) HandleGoogleOAuth(c *fiber.Ctx) error {
	// Check if the state and code are present in the query params
	state := c.Query("state")
	code := c.Query("code")

	if state == "" || code == "" {
		// Start the OAuth flow
		stateToken, err := h.TokenUtil.GenerateOpaqueToken(h.TokenUtil.Config.TokenLength)
		if err != nil {
			return c.Status(fiber.StatusInternalServerError).SendString("Failed to generate state")
		}

		// Store the state value in the cookie with minimal allocations
		c.Cookie(&fiber.Cookie{
			Name:     "oauth_state",
			Value:    stateToken,
			Path:     "/",
			HTTPOnly: true,
		})

		// Generate the AuthCodeURL
		url := h.AuthService.OAuthConfig.GoogleOAuth.AuthCodeURL(stateToken)
		return c.Redirect(url, fiber.StatusSeeOther)
	}

	// Validate the state parameter
	storedState := c.Cookies("oauth_state", "")
	if state != storedState {
		return c.Status(fiber.StatusUnauthorized).SendString("Invalid state parameter")
	}

	// Create a context with a timeout, reusing the context from Fiber if possible
	ctx, cancel := context.WithTimeout(c.UserContext(), 5*time.Second)
	defer cancel()

	// Exchange the code for a token
	otoken, err := h.AuthService.OAuthConfig.GoogleOAuth.Exchange(ctx, code)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).SendString("Failed to exchange code for token")
	}

	client := h.AuthService.OAuthConfig.GoogleOAuth.Client(ctx, otoken)

	// Create the request with context
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, GOOGLE_USER_INFO_URL, nil)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).SendString("Failed to create request")
	}

	// Make the request
	resp, err := client.Do(req)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).SendString("Failed to get user info")
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return c.Status(fiber.StatusInternalServerError).SendString("Failed to get user info")
	}

	// Use sonic's decoder to decode directly from the response body
	var userInfo dto.OAuthGoogleUser
	if err := sonic.ConfigFastest.NewDecoder(resp.Body).Decode(&userInfo); err != nil {
		return c.Status(fiber.StatusInternalServerError).SendString("Failed to parse user info")
	}

	// Save or update the OAuth user
	user, err := h.AuthService.SaveOAuthUser("google", userInfo.ID, userInfo.Email, userInfo.Name)
	if err != nil {
		h.Logger.Warn("OAuth Login failed", zap.Error(err))
		return c.Status(fiber.StatusUnauthorized).JSON(SimpleErrorResponse{Error: "Failed to login with Google"})
	}

	// Generate and save the token
	token, err := h.TokenService.GenerateAndSaveToken(user.UserID)
	if err != nil {
		h.Logger.Error("Failed to generate token for Google login", zap.Error(err))
		return c.Status(fiber.StatusInternalServerError).JSON(SimpleErrorResponse{Error: "Failed to generate token"})
	}

	// Generate the login cookie
	loginCookie := h.TokenUtil.GenerateLoginCookie(token)
	c.Cookie(&loginCookie)

	h.Logger.Info("Google user logged in successfully", zap.Int("userID", user.UserID))

	// Build the redirect URL efficiently
	customRedirectUrl := c.Query("returnUrl", "")
	redirectURL := h.AuthService.OAuthConfig.FrontendURL
	if customRedirectUrl == "" {
		redirectURL += "/mypage"
	} else {
		redirectURL += customRedirectUrl
	}

	return c.Redirect(redirectURL)
}

func (h *AuthHandler) HandleKakaoOAuth(c *fiber.Ctx) error {
	// Check if the state and code are present in the query params
	state := c.Query("state")
	code := c.Query("code")

	if state == "" || code == "" {
		// Start the OAuth flow
		stateToken, err := h.TokenUtil.GenerateOpaqueToken(h.TokenUtil.Config.TokenLength)
		if err != nil {
			return c.Status(fiber.StatusInternalServerError).SendString("Failed to generate state")
		}

		// Store the state value in the cookie with minimal allocations
		c.Cookie(&fiber.Cookie{
			Name:     "oauth_state",
			Value:    stateToken,
			Path:     "/",
			HTTPOnly: true,
		})

		// Generate the AuthCodeURL
		url := h.AuthService.OAuthConfig.GoogleOAuth.AuthCodeURL(stateToken)
		return c.Redirect(url, fiber.StatusSeeOther)
	}

	// Validate the state parameter
	storedState := c.Cookies("oauth_state")
	if state != storedState {
		return c.Status(fiber.StatusUnauthorized).SendString("Invalid state parameter")
	}

	// Create a context with a timeout, reusing the context from Fiber if possible
	ctx, cancel := context.WithTimeout(c.UserContext(), 5*time.Second)
	defer cancel()

	// Exchange the code for a token
	otoken, err := h.AuthService.OAuthConfig.KakaoOAuth.Exchange(ctx, code)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).SendString("Failed to exchange code for kakao token")
	}

	client := h.AuthService.OAuthConfig.KakaoOAuth.Client(ctx, otoken)

	// Create the request with context
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, KAKAO_USER_INFO_URL, nil)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).SendString("Failed to create request")
	}

	// Make the request
	resp, err := client.Do(req)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).SendString("Failed to get user info")
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return c.Status(fiber.StatusInternalServerError).SendString("Failed to get user info")
	}

	var userInfo dto.OAuthKakaoUser
	if err := sonic.ConfigFastest.NewDecoder(resp.Body).Decode(&userInfo); err != nil {
		return c.Status(fiber.StatusInternalServerError).SendString("Failed to parse user info")
	}

	user, err := h.AuthService.SaveOAuthUser("kakao", strconv.FormatInt(userInfo.ID, 10), userInfo.KakaoAccount.Email, userInfo.KakaoAccount.Profile.Nickname)
	if err != nil {
		h.Logger.Warn("OAuth Kakao Login failed", zap.Error(err))
		return c.Status(fiber.StatusUnauthorized).JSON(SimpleErrorResponse{Error: "Failed to login with Google"})
	}

	token, err := h.TokenService.GenerateAndSaveToken(user.UserID)
	if err != nil {
		h.Logger.Error("Failed to generate token for kakao login", zap.Error(err))
		return c.Status(fiber.StatusInternalServerError).JSON(SimpleErrorResponse{Error: "Failed to generate token"})
	}

	// Generate the login cookie
	loginCookie := h.TokenUtil.GenerateLoginCookie(token)
	c.Cookie(&loginCookie)

	h.Logger.Info("Kakao user logged in successfully", zap.Int("userID", user.UserID))

	// Build the redirect URL efficiently
	customRedirectUrl := c.Query("returnUrl", "")
	redirectURL := h.AuthService.OAuthConfig.FrontendURL
	if customRedirectUrl == "" {
		redirectURL += "/mypage"
	} else {
		redirectURL += customRedirectUrl
	}

	return c.Redirect(redirectURL)
}

func (h *AuthHandler) HandleNaverOAuth(c *fiber.Ctx) error {
	state := c.Query("state")
	code := c.Query("code")

	if state == "" || code == "" {
		// Start the OAuth flow
		stateToken, err := h.TokenUtil.GenerateOpaqueToken(h.TokenUtil.Config.TokenLength)
		if err != nil {
			return c.Status(fiber.StatusInternalServerError).SendString("Failed to generate state")
		}

		// Store the state value in the cookie with minimal allocations
		c.Cookie(&fiber.Cookie{
			Name:     "oauth_state",
			Value:    stateToken,
			Path:     "/",
			HTTPOnly: true,
		})

		// Generate the AuthCodeURL
		url := h.AuthService.OAuthConfig.GoogleOAuth.AuthCodeURL(stateToken)
		return c.Redirect(url, fiber.StatusSeeOther)
	}

	// Validate the state parameter
	storedState := c.Cookies("oauth_state", "")
	if state != storedState {
		return c.Status(fiber.StatusUnauthorized).SendString("Invalid state parameter")
	}

	// Create a context with a timeout, reusing the context from Fiber if possible
	ctx, cancel := context.WithTimeout(c.UserContext(), 5*time.Second)
	defer cancel()

	otoken, err := h.AuthService.OAuthConfig.NaverOAuth.Exchange(ctx, code)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).SendString("Failed to exchange code for token")
	}

	client := h.AuthService.OAuthConfig.NaverOAuth.Client(context.Background(), otoken)

	// Create the request with context
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, NAVER_USER_INFO_URL, nil)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).SendString("Failed to create naver oauth request")
	}

	// Make the request
	resp, err := client.Do(req)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).SendString("Failed to get user info")
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return c.Status(fiber.StatusInternalServerError).SendString("Failed to get user info")
	}

	var userInfo dto.OAuthNaverUser
	if err := sonic.ConfigFastest.NewDecoder(resp.Body).Decode(&userInfo); err != nil {
		return c.Status(fiber.StatusInternalServerError).SendString("Failed to parse user info")
	}

	user, err := h.AuthService.SaveOAuthUser("naver", userInfo.Response.ID, userInfo.Response.Email, userInfo.Response.Nickname)
	if err != nil {
		h.Logger.Warn("OAuth Login failed", zap.Error(err))
		return c.Status(fiber.StatusUnauthorized).JSON(SimpleErrorResponse{Error: "Failed to login with Naver"})
	}

	token, err := h.TokenService.GenerateAndSaveToken(user.UserID)
	if err != nil {
		h.Logger.Error("Failed to generate token for Naver login", zap.Error(err))
		return c.Status(fiber.StatusInternalServerError).JSON(SimpleErrorResponse{Error: "Failed to generate token"})
	}

	// Generate the login cookie
	loginCookie := h.TokenUtil.GenerateLoginCookie(token)
	c.Cookie(&loginCookie)

	h.Logger.Info("Naver user logged in successfully", zap.Int("userID", user.UserID))

	// Build the redirect URL efficiently
	customRedirectUrl := c.Query("returnUrl", "")
	redirectURL := h.AuthService.OAuthConfig.FrontendURL
	if customRedirectUrl == "" {
		redirectURL += "/mypage"
	} else {
		redirectURL += customRedirectUrl
	}

	return c.Redirect(redirectURL)
}

func (h *AuthHandler) HandleGitHubOAuth(c *fiber.Ctx) error {
	// Check if the state and code are present in the query params
	state := c.Query("state")
	code := c.Query("code")

	if state == "" || code == "" {
		// Start the OAuth flow
		state, err := h.TokenUtil.GenerateOpaqueToken(h.TokenUtil.Config.TokenLength)
		if err != nil {
			return c.Status(fiber.StatusInternalServerError).SendString("Failed to generate state")
		}

		// Store the state value in the session or a temporary store
		c.Cookie(&fiber.Cookie{
			Name:  "oauth_state",
			Value: state,
		})

		url := h.AuthService.OAuthConfig.GitHubOAuth.AuthCodeURL(state)
		c.Status(fiber.StatusSeeOther)
		return c.Redirect(url)
	}

	// Validate the state parameter
	storedState := c.Cookies("oauth_state")
	if state != storedState {
		return c.Status(fiber.StatusUnauthorized).SendString("Invalid state parameter")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Exchange the code for a token
	otoken, err := h.AuthService.OAuthConfig.GitHubOAuth.Exchange(ctx, code)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).SendString(err.Error())
	}

	client := h.AuthService.OAuthConfig.GitHubOAuth.Client(ctx, otoken)

	// Fetch user information from GitHub API
	response, err := client.Get("https://api.github.com/user")
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).SendString(err.Error())
	}
	defer response.Body.Close()

	body, err := io.ReadAll(response.Body)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).SendString(err.Error())
	}

	var userInfo struct {
		ID        int64  `json:"id"`
		Login     string `json:"login"`
		AvatarURL string `json:"avatar_url"`
		Email     string `json:"email"`
	}
	if err := sonic.Unmarshal(body, &userInfo); err != nil {
		return c.Status(fiber.StatusInternalServerError).SendString(err.Error())
	}

	// If Email is empty, we may need to fetch it separately because GitHub doesn't always include it in the main user profile response.
	if userInfo.Email == "" {
		emailResp, err := client.Get("https://api.github.com/user/emails")
		if err != nil {
			return c.Status(fiber.StatusInternalServerError).SendString(err.Error())
		}
		defer emailResp.Body.Close()

		emailBody, err := io.ReadAll(emailResp.Body)
		if err != nil {
			return c.Status(fiber.StatusInternalServerError).SendString(err.Error())
		}

		var emails []struct {
			Email    string `json:"email"`
			Primary  bool   `json:"primary"`
			Verified bool   `json:"verified"`
		}
		if err := sonic.Unmarshal(emailBody, &emails); err != nil {
			return c.Status(fiber.StatusInternalServerError).SendString(err.Error())
		}

		// Look for the primary and verified email
		for _, email := range emails {
			if email.Primary && email.Verified {
				userInfo.Email = email.Email
				break
			}
		}
	}

	user, err := h.AuthService.SaveOAuthUser("github", strconv.FormatInt(userInfo.ID, 10), userInfo.Email, userInfo.Login)
	if err != nil {
		h.Logger.Warn("OAuth GitHub Login failed", zap.Error(err))
		return c.Status(fiber.StatusUnauthorized).JSON(SimpleErrorResponse{Error: "Failed to login with GitHub"})
	}

	token, err := h.TokenService.GenerateAndSaveToken(user.UserID)
	if err != nil {
		h.Logger.Error("Failed to generate token for GitHub login", zap.Error(err))
		return c.Status(fiber.StatusInternalServerError).JSON(SimpleErrorResponse{Error: "Failed to generate token"})
	}

	cookie := h.TokenUtil.GenerateLoginCookie(token)
	c.Cookie(&cookie)

	h.Logger.Info("GitHub user logged in successfully", zap.Int("userID", user.UserID))

	// Build the redirect URL efficiently
	customRedirectUrl := c.Query("returnUrl", "")
	redirectURL := h.AuthService.OAuthConfig.FrontendURL
	if customRedirectUrl == "" {
		redirectURL += "/mypage"
	} else {
		redirectURL += customRedirectUrl
	}

	return c.Redirect(redirectURL)
}

// HandleOAuthProvider handles OAuth login for different providers.
//
// @Summary OAuth login handler
// @Description Handles OAuth authentication flow for supported providers (Google, Kakao, Naver)
// @ID handle-oauth-provider
// @Tags auth
// @Accept json
// @Produce json
// @Param provider path string true "OAuth provider (google, kakao, naver)"
// @Param state query string false "OAuth state parameter"
// @Param code query string false "OAuth authorization code"
// @Param mobileToken query string false "Access token from mobile client"
// @Param returnUrl query string false "Redirect URL after successful authentication"
// @Security
// @Success 200 {object} map[string]string "User successfully authenticated and redirected"
// @Failure 400 {object} map[string]string "Invalid provider or request parameters"
// @Failure 401 {object} map[string]string "Unauthorized due to invalid state or authentication failure"
// @Failure 500 {object} map[string]string "Internal server error during authentication"
// @Router /api/v1/auth/oauth/{provider} [get]
func (h *AuthHandler) HandleOAuthProvider(provider string) fiber.Handler {
	return func(c *fiber.Ctx) error {
		return h.handleOAuth(c, provider)
	}
}
func (h *AuthHandler) handleOAuth(c *fiber.Ctx, provider string) error {
	var (
		oauthConfig *oauth2.Config
		userInfoURL string
		oauthUser   dto.OAuthUser // interface
	)

	// provider별 설정
	switch provider {
	case "google":
		oauthConfig = h.AuthService.OAuthConfig.GoogleOAuth
		userInfoURL = GOOGLE_USER_INFO_URL
		oauthUser = &dto.OAuthGoogleUser{}
	case "kakao":
		oauthConfig = h.AuthService.OAuthConfig.KakaoOAuth
		userInfoURL = KAKAO_USER_INFO_URL
		oauthUser = &dto.OAuthKakaoUser{}
	case "naver":
		oauthConfig = h.AuthService.OAuthConfig.NaverOAuth
		userInfoURL = NAVER_USER_INFO_URL
		oauthUser = &dto.OAuthNaverUser{}
	default:
		return c.Status(fiber.StatusBadRequest).SendString("Unsupported provider")
	}

	// 5초 타임아웃 컨텍스트 생성
	ctx, cancel := context.WithTimeout(c.UserContext(), 5*time.Second)
	defer cancel()

	var otoken *oauth2.Token

	// mobileToken이 있으면 모바일 플로우, 없으면 웹 플로우
	mobileToken := c.Query("mobileToken", "")
	if mobileToken != "" {
		// 모바일: 이미 소셜 로그인에서 받은 access token 사용
		otoken = &oauth2.Token{
			AccessToken: mobileToken,
			TokenType:   "Bearer", // TODO: check if all providers are same
		}
	} else {
		// 웹: OAuth Code Flow 진행
		state := c.Query("state", "")
		code := c.Query("code", "")

		// 아직 code가 없으면 OAuth 플로우 시작 (state 생성, 쿠키에 저장, 리다이렉트)
		if state == "" || code == "" {
			stateToken, err := h.TokenUtil.GenerateOpaqueToken(h.TokenUtil.Config.TokenLength)
			if err != nil {
				return c.Status(fiber.StatusInternalServerError).SendString("Failed to generate state")
			}

			c.Cookie(&fiber.Cookie{
				Name:     "oauth_state",
				Value:    stateToken,
				Path:     "/",
				HTTPOnly: true,
			})
			return c.Redirect(oauthConfig.AuthCodeURL(stateToken), fiber.StatusSeeOther)
		}

		// state 검증
		storedState := c.Cookies("oauth_state", "")
		if state != storedState {
			return c.Status(fiber.StatusUnauthorized).SendString("Invalid state parameter")
		}

		var err error
		otoken, err = oauthConfig.Exchange(ctx, code)
		if err != nil {
			return c.Status(fiber.StatusInternalServerError).SendString("Failed to exchange code for token")
		}
	}

	// 공통: oauth token(otoken)을 이용해 사용자 정보 요청
	client := oauthConfig.Client(ctx, otoken)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, userInfoURL, nil)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).SendString("Failed to create request")
	}

	resp, err := client.Do(req)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).SendString("Failed to get user info")
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return c.Status(fiber.StatusInternalServerError).SendString("Failed to get user info")
	}

	// JSON 응답을 각 DTO (OAuthUser 인터페이스 구현체)로 디코딩
	if err := sonic.ConfigFastest.NewDecoder(resp.Body).Decode(oauthUser); err != nil {
		return c.Status(fiber.StatusInternalServerError).SendString("Failed to parse user info")
	}

	// 인터페이스 메서드를 사용해 공통 정보 추출
	id := oauthUser.GetID()
	email := oauthUser.GetEmail()
	name := oauthUser.GetName()

	// OAuth 유저 저장 또는 업데이트
	user, err := h.AuthService.SaveOAuthUser(provider, id, email, name)
	if err != nil {
		h.Logger.Warn("OAuth Login failed", zap.String("provider", provider), zap.Error(err))
		return c.Status(fiber.StatusUnauthorized).JSON(SimpleErrorResponse{Error: "Failed to login with " + provider})
	}

	// 서비스 전용 로그인 토큰 생성 및 저장
	token, err := h.TokenService.GenerateAndSaveToken(user.UserID)
	if err != nil {
		h.Logger.Error("Failed to generate token for login", zap.String("provider", provider), zap.Error(err))
		return c.Status(fiber.StatusInternalServerError).JSON(SimpleErrorResponse{Error: "Failed to generate token"})
	}

	// 로그인 쿠키 생성 및 설정 (React Native WebView에서도 쿠키 사용 가능)
	loginCookie := h.TokenUtil.GenerateLoginCookie(token)
	c.Cookie(&loginCookie)

	h.Logger.Info("OAuth2 User logged in successfully", zap.String("provider", provider), zap.Int("userID", user.UserID))
	h.LoginCounter.Inc() // 로그인 카운터 증가

	// 리다이렉트 URL 빌드 (returnUrl 쿼리 파라미터가 있으면 사용)
	customRedirectUrl := c.Query("returnUrl", "")
	redirectURL := h.AuthService.OAuthConfig.FrontendURL
	if customRedirectUrl == "" {
		redirectURL += "/mypage"
	} else {
		redirectURL += customRedirectUrl
	}

	return c.Redirect(redirectURL)
}

// HandleLogout handles user logout.
//
// @Summary Logout user
// @Description Logs out the authenticated user by clearing the session and authentication token.
// @ID handle-logout
// @Tags auth
// @Accept json
// @Produce json
// @Security ApiKeyAuth
// @Success 200 {object} map[string]string "Logged out successfully"
// @Failure 500 {object} map[string]string "Internal server error during logout"
// @Router /api/v1/auth/logout [post]
func (h *AuthHandler) HandleLogout(c *fiber.Ctx) error {
	// Retrieve user ID from context or session
	userID, ok := c.Locals("userID").(int)
	if !ok {
		// If user ID is missing, continue with logout without error
		// Log the occurrence for internal tracking
		h.Logger.Warn("UserID missing in session during logout")
	}

	token := c.Cookies("token")

	if token != "" {
		// Attempt to delete the token from the database
		if err := h.TokenService.DeleteOpaqueToken(userID, token); err != nil {
			// Log the error but do not disrupt the logout process
			h.Logger.Warn("Failed to delete session token", zap.Int("userID", userID), zap.Error(err))
		}
	}

	// Clear the authentication cookie
	cookie := h.TokenUtil.ClearLoginCookie()
	c.Cookie(&cookie)

	h.Logger.Info("User logged out successfully", zap.Int("userID", userID))

	// Return a logout success response regardless of server-side token deletion status
	return c.JSON(fiber.Map{"message": "Logged out successfully"})
}

// Send Verification Email godoc
//
// @Summary		Send verification email
// @Description	This endpoint triggers sending a verification email to the user.
// @Description	It checks if the email is already registered in the system.
// @Description	If the email is already in use, it returns an error.
// @Description	If the email is not in use, it asynchronously sends a verification email to the user.
// @Description	The operation of sending the email does not block the API response, making use of a goroutine for asynchronous execution.
// @ID			send-verification-email
// @Tags		auth
// @Accept		json
// @Produce	json
// @Security
// @Param		email	formData	string	true	"User Email"
// @Success	200	"Email sending initiated successfully"
// @Failure	409	{object}	map[string]interface{}	"Email already registered"
// @Failure	500	{object}	map[string]interface{}	"An unexpected error occurred"
// @Router		/api/v1/auth/send-verification-email [post]
func (h *AuthHandler) HandleSendVerificationEmail(c *fiber.Ctx) error {
	userEmail := c.FormValue("email")
	userEmail = strings.ToLower(userEmail)
	_, err := h.UserService.GetUserByEmail(userEmail)
	if err == nil {
		// If GetUserByEmail does not return an error, it means the email is already in use
		return c.Status(fiber.StatusConflict).JSON(SimpleErrorResponse{Error: "Email already registered"})
	} else if err != sql.ErrNoRows {
		// if db couldn't find a user, then it's valid. other errors are bad.
		h.Logger.Error("Unexpected error occurred while checking email", zap.Error(err))
		return c.Status(fiber.StatusInternalServerError).JSON(SimpleErrorResponse{Error: "An unexpected error occurred"})
	}

	// No matter if it's verified, send again.
	// Check if there's already a verified token for this user
	// verified, err := services.IsTokenVerified(userEmail)
	// if err != nil {
	// 	return c.Status(fiber.StatusInternalServerError).JSON(SimpleErrorResponse{Error: "Failed to check verification status"})
	// }
	// if verified {
	// 	return c.Status(fiber.StatusBadRequest).JSON(SimpleErrorResponse{Error: "Email already verified"})
	// }

	// token, err := services.GenerateAndSaveSignUpToken(userEmail)
	// if err != nil {
	// 	return c.Status(fiber.StatusInternalServerError).JSON(SimpleErrorResponse{Error: "Failed to generate token"})
	// }

	// Use a goroutine to send the email without blocking
	go func(email string) {
		if strings.HasSuffix(email, "@naver.com") { // endsWith
			exist, _ := h.AuthService.VerifyNaverEmail(email)
			if !exist {
				h.Logger.Warn("No such email found on Naver", zap.String("email", email))
				return
			}
		}
		token, err := h.TokenService.GenerateAndSaveSignUpToken(email)
		if err != nil {
			h.Logger.Error("Failed to generate token", zap.String("email", email), zap.Error(err))
			return
		}

		err = h.SmtpService.SendVerificationEmail(email, token)
		if err != nil {
			h.Logger.Error("Failed to send verification email", zap.String("email", email), zap.Error(err))
			return
		}
	}(userEmail)

	return c.SendStatus(fiber.StatusOK)
}

// Validate Token godoc
//
// @Summary		Validate token
// @Description	This endpoint is responsible for validating a user's token.
// @Description	It checks the token's validity against the provided email.
// @Description	If the token is invalid or expired, it returns an error.
// @Description	On successful validation, it returns a success status.
// @ID			validate-token
// @Tags		auth
// @Accept		json
// @Produce	json
// @Param		token	formData	string	true	"Token for validation"
// @Param		email	formData	string	true	"User's email associated with the token"
// @Security
// @Success	200	"Token validated successfully"
// @Failure	400	{object}	map[string]interface{}	"Invalid or expired token"
// @Failure	500	{object}	map[string]interface{}	"Error validating token"
// @Router		/api/v1/auth/validate-token [post]
func (h *AuthHandler) HandleValidateToken(c *fiber.Ctx) error {
	token := c.FormValue("token")
	email := c.FormValue("email")

	valid, err := h.TokenService.ValidateToken(token, email)
	if err != nil {
		// If err is not nil, it could be a database error or token not found
		return c.Status(fiber.StatusInternalServerError).JSON(SimpleErrorResponse{Error: "Error validating token"})
	}
	if !valid {
		// Handle both not found and expired cases
		return c.Status(fiber.StatusBadRequest).JSON(SimpleErrorResponse{Error: "Invalid or expired token"})
	}

	return c.SendStatus(fiber.StatusOK)
}

// Request Reset Password godoc
//
// @Summary		Request password reset
// @Description	This endpoint initiates the password reset process for a user.
// @Description	It generates a password reset token and sends a reset password email to the user.
// @Description	The email sending process is executed in a non-blocking manner using a goroutine.
// @Description	If there is an issue generating the token or sending the email, it returns an error.
// @ID			request-reset-password
// @Tags		auth
// @Accept		json
// @Produce	json
// @Param		email	formData	string	true	"User's email address for password reset"
// @Security
// @Success	200	"Password reset request initiated successfully"
// @Failure	500	{object}	map[string]interface{}	"Failed to request reset password"
// @Router		/api/v1/auth/request-reset-password [post]
func (h *AuthHandler) HandleRequestResetPassword(c *fiber.Ctx) error {
	email := c.FormValue("email")

	// Generate the password reset token
	token, err := h.AuthService.GeneratePasswordResetToken(email)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(SimpleErrorResponse{Error: "Failed to request reset password"})
	}

	// Use a goroutine to send the email without blocking
	go func(email string) {

		// Send the reset email
		err = h.SmtpService.SendPasswordResetEmail(email, token)
		if err != nil {
			// cannot respond to the client at this point
			h.Logger.Error("Error sending reset email", zap.String("email", email), zap.Error(err))
			return
		}
	}(email)

	return c.SendStatus(fiber.StatusOK)
}

// Reset Password godoc
//
// @Summary		Reset password
// @Description	This endpoint allows a user to reset their password using a valid token.
// @Description	The token is typically obtained from a password reset email.
// @Description	If the token is invalid or the reset fails, it returns an error.
// @Description	On successful password reset, it returns a success status.
// @ID			reset-password
// @Tags		auth
// @Accept		json
// @Produce	json
// @Param		token		formData	string	true	"Password reset token"
// @Param		password	formData	string	true	"New password"
// @Security
// @Success	200	"Password reset successfully"
// @Failure	500	{object}	map[string]interface{}	"Failed to reset password"
// @Router		/api/v1/auth/reset-password [post]
func (h *AuthHandler) HandleResetPassword(c *fiber.Ctx) error {
	token := c.FormValue("token")
	newPassword := c.FormValue("password")

	err := h.AuthService.ResetPassword(token, newPassword)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(SimpleErrorResponse{Error: "Failed to reset password"})
	}

	return c.SendStatus(fiber.StatusOK)
}

package middlewares

import (
	"chulbong-kr/database"
	"database/sql"
	"log"
	"time"

	"github.com/gofiber/fiber/v2"
)

// TODO: Change to cookie authentication
// c.Cookie(&fiber.Cookie{
//     Name:     "token",
//     Value:    "token",
//     Expires:  time.Now().Add(24 * time.Hour),
//     HttpOnly: true,
//     Secure:   true, // if your site uses HTTPS
//     SameSite: "Lax", // or "Strict" depending on your requirements
// })

var TOKEN_COOKIE string

// AuthMiddleware checks for a valid opaque token in the Authorization header
func AuthMiddleware(c *fiber.Ctx) error {
	// check for the cookie
	jwtCookie := c.Cookies(TOKEN_COOKIE)
	if jwtCookie == "" {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "No authorization token provided"})
	}
	token := jwtCookie

	// // Check if the Authorization header is provided
	// if authHeader != "" {
	// 	// Split the Authorization header to extract the token
	// 	parts := strings.SplitN(authHeader, " ", 2)
	// 	if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") {
	// 		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "Authorization header format must be Bearer {token}"})
	// 	}
	// 	token = parts[1] // The actual token part
	// } else {

	// }

	query := `SELECT UserID, ExpiresAt FROM OpaqueTokens WHERE OpaqueToken = ?`
	var userID int
	var expiresAt time.Time
	err := database.DB.QueryRow(query, token).Scan(&userID, &expiresAt)

	// Adjust the error check to specifically look for no rows found, indicating an invalid or expired token.
	if err == sql.ErrNoRows {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "Invalid or expired token"})
	} else if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Server error"})
	}

	if time.Now().After(expiresAt) {
		// // Token has expired, delete it
		// delQuery := `DELETE FROM OpaqueTokens WHERE OpaqueToken = ?`
		// if _, delErr := database.DB.Exec(delQuery, token); delErr != nil {
		// 	// Log the error; decide how you want to handle the failure of deleting an expired token
		// 	fmt.Println("Failed to delete expired token:", delErr)
		// }
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "Token expired"})
	}

	// Fetch UserID and Username based on Email
	userQuery := `SELECT Username, Email FROM Users WHERE UserID = ?`
	var username string
	var email string
	err = database.DB.QueryRow(userQuery, userID).Scan(&username, &email)
	if err != nil {
		if err == sql.ErrNoRows {
			return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "User not found"})
		}
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Server error fetching user details"})
	}

	// Store UserID, Username and Email in locals for use in subsequent handlers
	c.Locals("userID", userID)
	c.Locals("username", username)
	c.Locals("email", email)

	log.Printf("[DEBUG] Authenticated. %s", email)
	return c.Next()
}

// AdminOnly checks admin permission
func AdminOnly(c *fiber.Ctx) error {
	// Check for the cookie
	jwtCookie := c.Cookies(TOKEN_COOKIE)
	if jwtCookie == "" {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "No authorization token provided"})
	}

	// query to also fetch the user's role
	query := `
    SELECT u.UserID, u.Role, ot.ExpiresAt 
    FROM OpaqueTokens ot
    JOIN Users u ON ot.UserID = u.UserID
    WHERE ot.OpaqueToken = ?`

	var userID int
	var role string
	var expiresAt time.Time
	err := database.DB.QueryRow(query, jwtCookie).Scan(&userID, &role, &expiresAt)

	// Check for no rows found or other errors
	if err == sql.ErrNoRows {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "Invalid or expired token"})
	} else if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Server error"})
	}

	// Check if the token has expired
	if time.Now().After(expiresAt) {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "Token expired"})
	}

	// Check if the user role is not admin
	if role != "admin" {
		return c.Status(fiber.StatusForbidden).JSON(fiber.Map{"error": "Access denied"})
	}

	// Proceed to the next handler if the user is an admin
	return c.Next()
}

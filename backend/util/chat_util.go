package util

import (
	"fmt"
	"math/rand/v2"
	"net"
	"net/http"
	"strconv"
	"strings"

	"github.com/goccy/go-json"
	"github.com/gofiber/fiber/v2"
)

type ChatUtil struct {
	HTTPClient *http.Client
}

func NewChatUtil(httpClient *http.Client) *ChatUtil {
	return &ChatUtil{
		HTTPClient: httpClient,
	}
}

var adjectives = []string{
	"귀여운",     // Cute
	"멋진",      // Cool
	"착한",      // Kind
	"용감한",     // Brave
	"영리한",     // Clever
	"재미있는",    // Fun
	"행복한",     // Happy
	"사랑스러운",   // Lovely
	"기운찬",     // Energetic
	"빛나는",     // Shining
	"평화로운",    // Peaceful
	"신비로운",    // Mysterious
	"자유로운",    // Free
	"매력적인",    // Charming
	"섬세한",     // Delicate
	"우아한",     // Elegant
	"활발한",     // Lively
	"강인한",     // Strong
	"독특한",     // Unique
	"무서운",     // Scary
	"꿈꾸는",     // Dreamy
	"느긋한",     // Relaxed
	"열정적인",    // Passionate
	"소중한",     // Precious
	"신선한",     // Fresh
	"창의적인",    // Creative
	"우수한",     // Excellent
	"재치있는",    // Witty
	"감각적인",    // Sensual
	"흥미로운",    // Interesting
	"유명한",     // Famous
	"현명한",     // Wise
	"대담한",     // Bold
	"침착한",     // Calm
	"신속한",     // Swift
	"화려한",     // Gorgeous
	"정열적인",    // Passionate (Alternate)
	"끈기있는",    // Persistent
	"애정이 깊은",  // Affectionate
	"민첩한",     // Agile
	"빠른",      // Quick
	"조용한",     // Quiet
	"명랑한",     // Cheerful
	"정직한",     // Honest
	"용서하는",    // Forgiving
	"용기있는",    // Courageous
	"성실한",     // Sincere
	"호기심이 많은", // Curious
	"겸손한",     // Humble
	"관대한",     // Generous
}

// 9 names
var names = []string{
	"라이언", // Ryan
	"어피치", // Apeach
	"콘",   // Con
	"무지",  // Muzi
	"네오",  // Neo
	"프로도", // Frodo
	"제이지", // Jay-G
	"튜브",  // Tube
	"철봉",  // chulbong
}

// GenerateKoreanNickname generates random user nickname
func (c *ChatUtil) GenerateKoreanNickname() string {
	// Select a random
	adjective := adjectives[rand.IntN(len(adjectives))]

	name := names[rand.IntN(len(names))]
	randomValue := rand.Uint32()

	// Convert the uint32 to a hex string without padding
	shortUID := strconv.FormatUint(uint64(randomValue), 16)

	var builder strings.Builder
	builder.Grow(len(adjective) + len(name) + len(shortUID) + 4)

	builder.WriteString(adjective)
	builder.WriteString(" ")
	builder.WriteString(name)
	builder.WriteString(" [")
	builder.WriteString(shortUID)
	builder.WriteString("]")

	// possibilities for conflict
	// highly unlikely.
	// 25 * 9 * 16^8 (UUID first 8 characters)
	// UUID can conflict by root(16*8) = 65,536
	// return fmt.Sprintf("%s %s [%s]", adjective, name, shortUID)
	return builder.String()
}

// GenerateUsernameForAuth generates a simpler username format for OAuth authentication
// Format: "귀여운-무지-12145" (no spaces or brackets for database compatibility)
func (c *ChatUtil) GenerateUsernameForAuth() string {
	adjective := adjectives[rand.IntN(len(adjectives))]
	name := names[rand.IntN(len(names))]
	randomValue := rand.Uint32()

	// Convert to a smaller number for shorter usernames
	shortUID := strconv.FormatUint(uint64(randomValue%100000), 10)

	var builder strings.Builder
	builder.Grow(len(adjective) + len(name) + len(shortUID) + 2)

	builder.WriteString(adjective)
	builder.WriteString("-")
	builder.WriteString(name)
	builder.WriteString("-")
	builder.WriteString(shortUID)

	return builder.String()
}

func (cu *ChatUtil) GetUserIP(c *fiber.Ctx) string {
	if c == nil {
		return ""
	}

	clientIP := c.Get("Fly-Client-IP")
	if clientIP == "" {
		clientIP = c.Get("Fly-Client-Ip")
	}
	// If Fly-Client-IP is not found, fall back to X-Forwarded-For
	if clientIP == "" {
		clientIP = c.Get("X-Forwarded-For")
		if clientIP != "" {
			// X-Forwarded-For can contain multiple IPs, take the first one
			clientIP = strings.Split(clientIP, ",")[0]
		}
	}
	if clientIP == "" {
		clientIP = c.Get("X-Real-IP")
	}

	// If X-Forwarded-For is also empty, use c.IP() as the last resort
	if clientIP == "" {
		clientIP = c.IP()
	}

	// Validate and clean the IP address
	clientIP = strings.TrimSpace(clientIP)
	if net.ParseIP(clientIP) == nil {
		return ""
	}

	return clientIP
}

func (cu *ChatUtil) anonymizeIP(c *fiber.Ctx) string {
	ip := cu.GetUserIP(c)
	parts := strings.Split(ip, ".")
	if len(parts) >= 2 {
		return parts[0] + "." + parts[1] // Return only the first two segments
	}
	return ip // Fallback in case of unexpected format
}

func (cu *ChatUtil) CreateAnonymousID(c *fiber.Ctx) string {
	adjective := adjectives[rand.IntN(len(adjectives))]
	name := names[rand.IntN(len(names))]

	return fmt.Sprintf("%s%s#%s", adjective, name, cu.anonymizeIP(c)) // Combine nickname and IP
}

// NordAPIResponse represents the response structure from the Nord API
type NordAPIResponse struct {
	Country     string `json:"country"`
	CountryCode string `json:"country_code"`
	ISP         string `json:"isp"`
}

// TODO: alternative: ip-api.com
// TODO: https://github.com/oschwald/geoip2-golang
func (cu *ChatUtil) IsIPFromSouthKorea(ip string) (bool, error) {
	url := fmt.Sprintf("https://nordvpn.com/wp-admin/admin-ajax.php?action=get_user_info_data&ip=%s", ip)

	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return false, fmt.Errorf("creating request: %w", err)
	}

	req.Header.Add("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/125.0.0.0 Safari/537.36")
	req.Header.Add("X-Forwarded-For", GenerateRandomIP())

	// Create a new HTTP request
	resp, err := cu.HTTPClient.Do(req)
	if err != nil {
		return false, fmt.Errorf("executing request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return false, fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	var response NordAPIResponse
	if err := json.NewDecoder(resp.Body).Decode(&response); err != nil {
		return false, fmt.Errorf("unmarshalling response: %w", err)
	}

	// log.Printf("💕 ip: %s", response.Country)
	return response.CountryCode == "KR" && response.ISP != "Amazon.com", nil
}

// GenerateRandomIP generates a random IPv4 address
func GenerateRandomIP() string {
	ip := net.IPv4(byte(rand.IntN(256)), byte(rand.IntN(256)), byte(rand.IntN(256)), byte(rand.IntN(256)))
	return ip.String()
}

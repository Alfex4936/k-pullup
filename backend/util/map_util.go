package util

import (
	"math"

	"github.com/Alfex4936/tzf"

	iradix "github.com/hashicorp/go-immutable-radix/v2"
)

/*
북한 제외
극동: 경상북도 울릉군의 독도(獨島)로 동경 131° 52′20“, → 131.87222222
극서: 전라남도 신안군의 소흑산도(小黑山島)로 동경 125° 04′, → 125.06666667
극북: 강원도 고성군 현내면 송현진으로 북위 38° 27′00, → 38.45000000
극남: 제주도 남제주군 마라도(馬羅島)로 북위 33° 06′00" → 33.10000000
섬 포함 우리나라의 중심점은 강원도 양구군 남면 도촌리 산48번지
북위 38도 03분 37.5초, 동경 128도 02분 2.5초 → 38.05138889, 128.03388889
섬을 제외하고 육지만을 놓고 한반도의 중심점을 계산하면 북한에 위치한 강원도 회양군 현리 인근
북위(lon): 38도 39분 00초, 동경(lat) 127도 28분 55초 → 33.10000000, 127.48194444
대한민국
도분초: 37° 34′ 8″ N, 126° 58′ 36″ E
소수점 좌표: 37.568889, 126.976667
*/
// South Korea's bounding box
const (
	SouthKoreaMinLat  = 33.0
	SouthKoreaMaxLat  = 38.615
	SouthKoreaMinLong = 124.0
	SouthKoreaMaxLong = 132.0
)

// Tsushima (Uni Island) bounding box
const (
	TsushimaMinLat  = 34.080
	TsushimaMaxLat  = 34.708
	TsushimaMinLong = 129.164396
	TsushimaMaxLong = 129.4938
)

// Nagasaki bounding box
const (
	NagasakiMinLat  = 32.75
	NagasakiMaxLat  = 34.41
	NagasakiMinLong = 128.67
	NagasakiMaxLong = 131.07
)

// Fukuoka bounding box
const (
	FukuokaMinLat  = 33.14
	FukuokaMaxLat  = 33.88
	FukuokaMinLong = 129.10
	FukuokaMaxLong = 130.64
)

const RadiusOfEarthMeters float64 = 6370986
const KoreaTimeZone = "Asia/Seoul"

const (
	// WGS84 Ellipsoid constants
	majorAxisWGS84 = 6378137.0
	fWGS84         = 1.0 / 298.257223563

	// TM Projection Parameters (Korea Central Belt 2010 / Kakao)
	centerLat     = 38.0 * (math.Pi / 180.0)
	centerLon     = 127.0 * (math.Pi / 180.0)
	falseEasting  = 200000.0
	falseNorthing = 500000.0
	scaleFactor   = 1.0 // k0

	// KakaoMap specific scale
	kakaoScale = 2.5

	radiansPerDegree = math.Pi / 180.0
	degreesPerRadian = 180.0 / math.Pi
)

// Map for provinces and major regions
var provinceMap = map[string]struct{}{
	"서울특별시":   {},
	"부산광역시":   {},
	"대구광역시":   {},
	"인천광역시":   {},
	"광주광역시":   {},
	"대전광역시":   {},
	"울산광역시":   {},
	"세종특별자치시": {},
	"경기도":     {},
	"강원특별자치도": {},
	"충청북도":    {},
	"충청남도":    {},
	"전북특별자치도": {},
	"전라남도":    {},
	"경상북도":    {},
	"경상남도":    {},
	"제주특별자치도": {},
}

// Map for cities, districts (구), and counties (군)
var cityMap = map[string]struct{}{
	// 서울특별시
	"종로구":  {},
	"중구":   {},
	"용산구":  {},
	"성동구":  {},
	"광진구":  {},
	"동대문구": {},
	"중랑구":  {},
	"성북구":  {},
	"강북구":  {},
	"도봉구":  {},
	"노원구":  {},
	"은평구":  {},
	"서대문구": {},
	"마포구":  {},
	"양천구":  {},
	"강서구":  {},
	"구로구":  {},
	"금천구":  {},
	"영등포구": {},
	"동작구":  {},
	"관악구":  {},
	"서초구":  {},
	"강남구":  {},
	"송파구":  {},
	"강동구":  {},

	// 부산광역시
	//"중구":   {}, // dup
	"서구":   {},
	"동구":   {},
	"영도구":  {},
	"부산진구": {},
	"진구":   {}, // 부산진구
	"동래구":  {},
	"남구":   {},
	"북구":   {},
	"해운대구": {},
	"사하구":  {},
	"금정구":  {},
	//"강서구":  {}, // dup
	"연제구": {},
	"수영구": {},
	"사상구": {},
	"기장군": {},

	// 대구광역시
	//"중구":  {}, // dup
	//"동구":  {}, // dup
	//"서구":  {}, // dup
	//"남구":  {}, // dup
	//"북구":  {}, // dup
	"수성구": {},
	"달서구": {},
	"달성군": {},
	"군위군": {},

	// 인천광역시
	// "중구":   {}, // dup
	// "동구":   {}, // dup
	"미추홀구": {},
	"연수구":  {},
	"남동구":  {},
	"부평구":  {},
	"계양구":  {},
	// "서구":   {}, // dup
	"강화군": {},
	"옹진군": {},

	// 광주광역시
	// "동구":  {}, // dup
	// "서구":  {}, // dup
	// "남구":  {}, // dup
	// "북구":  {}, // dup
	"광산구": {},

	// 대전광역시
	// "중구":  {}, // dup
	// "서구":  {}, // dup
	// "동구":  {}, // dup
	"유성구": {},
	"대덕구": {},

	// 울산광역시
	// "중구":  {}, // dup
	// "남구":  {}, // dup
	// "동구":  {}, // dup
	// "북구":  {}, // dup
	"울주군": {},

	// 세종특별자치시
	"조치원읍": {},
	"연기면":  {},
	"연동면":  {},
	"부강면":  {},
	"금남면":  {},
	"장군면":  {},
	"연서면":  {},
	"전의면":  {},
	"전동면":  {},
	"소정면":  {},
	"한솔동":  {},
	"새롬동":  {},
	"나성동":  {},
	"다정동":  {},
	"도담동":  {},
	"어진동":  {},
	"해밀동":  {},
	"아름동":  {},
	"종촌동":  {},
	"고운동":  {},
	"보람동":  {},
	"대평동":  {},
	"소담동":  {},
	"반곡동":  {},

	// 경기도
	"수원시":  {},
	"성남시":  {},
	"의정부시": {},
	"안양시":  {},
	"부천시":  {},
	"광명시":  {},
	"동두천시": {},
	"평택시":  {},
	"안산시":  {},
	"고양시":  {},
	"과천시":  {},
	"구리시":  {},
	"남양주시": {},
	"오산시":  {},
	"시흥시":  {},
	"군포시":  {},
	"의왕시":  {},
	"하남시":  {},
	"용인시":  {},
	"파주시":  {},
	"이천시":  {},
	"안성시":  {},
	"김포시":  {},
	"화성시":  {},
	"광주시":  {},
	"양주시":  {},
	"포천시":  {},
	"여주시":  {},
	"연천군":  {},
	"가평군":  {},
	"양평군":  {},

	// 강원특별자치도
	"춘천시": {},
	"원주시": {},
	"강릉시": {},
	"동해시": {},
	"태백시": {},
	"속초시": {},
	"삼척시": {},
	"홍천군": {},
	"횡성군": {},
	"영월군": {},
	"평창군": {},
	"정선군": {},
	"철원군": {},
	"화천군": {},
	"양구군": {},
	"인제군": {},
	"고성군": {},
	"양양군": {},

	// 충청북도
	"청주시": {},
	"충주시": {},
	"제천시": {},
	"보은군": {},
	"옥천군": {},
	"영동군": {},
	"증평군": {},
	"진천군": {},
	"괴산군": {},
	"음성군": {},
	"단양군": {},

	// 충청남도
	"천안시": {},
	"공주시": {},
	"보령시": {},
	"아산시": {},
	"서산시": {},
	"논산시": {},
	"계룡시": {},
	"당진시": {},
	"금산군": {},
	"부여군": {},
	"서천군": {},
	"청양군": {},
	"홍성군": {},
	"예산군": {},
	"태안군": {},

	// 전북특별자치도
	"전주시": {},
	"군산시": {},
	"익산시": {},
	"정읍시": {},
	"남원시": {},
	"김제시": {},
	"완주군": {},
	"진안군": {},
	"무주군": {},
	"장수군": {},
	"임실군": {},
	"순창군": {},
	"고창군": {},
	"부안군": {},

	// 전라남도
	"목포시": {},
	"여수시": {},
	"순천시": {},
	"나주시": {},
	"광양시": {},
	"담양군": {},
	"곡성군": {},
	"구례군": {},
	"고흥군": {},
	"보성군": {},
	"화순군": {},
	"장흥군": {},
	"강진군": {},
	"해남군": {},
	"영암군": {},
	"무안군": {},
	"함평군": {},
	"영광군": {},
	"장성군": {},
	"완도군": {},
	"진도군": {},
	"신안군": {},

	// 경상북도
	"포항시": {},
	"경주시": {},
	"김천시": {},
	"안동시": {},
	"구미시": {},
	"영주시": {},
	"영천시": {},
	"상주시": {},
	"문경시": {},
	"경산시": {},
	"의성군": {},
	"청송군": {},
	"영양군": {},
	"영덕군": {},
	"청도군": {},
	"고령군": {},
	"성주군": {},
	"칠곡군": {},
	"예천군": {},
	"봉화군": {},
	"울진군": {},
	"울릉군": {},

	// 경상남도
	"창원시": {},
	"진주시": {},
	"통영시": {},
	"사천시": {},
	"김해시": {},
	"밀양시": {},
	"거제시": {},
	"양산시": {},
	"의령군": {},
	"함안군": {},
	"창녕군": {},
	// "고성군": {}, // dup
	"남해군": {},
	"하동군": {},
	"산청군": {},
	"함양군": {},
	"거창군": {},
	"합천군": {},

	// 제주특별자치도
	"제주시":  {},
	"서귀포시": {},
}

var (
	wConst = math.Atan(1) / 45 // Precomputed constant value

	provinceRadix *iradix.Tree[int] = iradix.New[int]()
	cityRadix     *iradix.Tree[int] = iradix.New[int]()
)

type MapUtil struct {
	TimeZoneFinder tzf.F
}

func NewMapUtil(finder tzf.F) *MapUtil {
	// Insert provinces into the radix tree
	for province := range provinceMap {
		provinceRadix, _, _ = provinceRadix.Insert([]byte(province), 1)
	}

	// Insert cities into the radix tree
	for city := range cityMap {
		cityRadix, _, _ = cityRadix.Insert([]byte(city), 1)
	}

	return &MapUtil{
		TimeZoneFinder: finder,
	}
}

// CalculateDistanceApproximately optimizes the Haversine formula for small distances
func CalculateDistanceApproximately(lat1, long1, lat2, long2 float64) float64 {
	// Convert degrees to radians
	const degToRad = math.Pi / 180
	lat1Rad := lat1 * degToRad
	lat2Rad := lat2 * degToRad
	deltaLat := (lat2 - lat1) * degToRad
	deltaLong := (long2 - long1) * degToRad

	// Calculate components
	x := deltaLong * math.Cos((lat1Rad+lat2Rad)*0.5)
	y := deltaLat

	// Approximate distance using the optimized formula
	return math.Sqrt(x*x+y*y) * RadiusOfEarthMeters
}

// distance calculates the distance between two geographic coordinates in meters
func distance(lat1, long1, lat2, long2 float64) float64 {
	var deltaLat = (lat2 - lat1) * (math.Pi / 180)
	var deltaLong = (long2 - long1) * (math.Pi / 180)
	var a = math.Sin(deltaLat/2)*math.Sin(deltaLat/2) +
		math.Cos(lat1*(math.Pi/180))*math.Cos(lat2*(math.Pi/180))*
			math.Sin(deltaLong/2)*math.Sin(deltaLong/2)
	var c = 2 * math.Atan2(math.Sqrt(a), math.Sqrt(1-a))
	return 6370986 * c // Earth radius in meters
}

// IsInSouthKorea checks if given latitude and longitude are within South Korea (roughly)
func IsInSouthKorea(lat, long float64) bool {
	// Check if within Tsushima (Uni Island) and return false if true
	if lat >= TsushimaMinLat && lat <= TsushimaMaxLat && long >= TsushimaMinLong && long <= TsushimaMaxLong {
		return false // The point is within Tsushima Island, not South Korea
	}

	// Check if within Nagasaki and return false if true
	if lat >= NagasakiMinLat && lat <= NagasakiMaxLat && long >= NagasakiMinLong && long <= NagasakiMaxLong {
		return false // The point is within Nagasaki, not South Korea
	}

	// Check if within Fukuoka and return false if true
	if lat >= FukuokaMinLat && lat <= FukuokaMaxLat && long >= FukuokaMinLong && long <= FukuokaMaxLong {
		return false // The point is within Fukuoka, not South Korea
	}

	// Check if within South Korea's bounding box
	return lat >= SouthKoreaMinLat && lat <= SouthKoreaMaxLat && long >= SouthKoreaMinLong && long <= SouthKoreaMaxLong
}

func (t *MapUtil) IsInSouthKoreaPrecisely(lat, lng float64) bool {
	// Get timezone name for the coordinates
	return t.TimeZoneFinder.GetTimezoneName(lng, lat) == KoreaTimeZone
}

// CONVERT ----------------------------------------------------------------
// WCONGNAMULCoord represents a coordinate in the WCONGNAMUL system.
type WCONGNAMULCoord struct {
	X float64 // X coordinate (Easting-like)
	Y float64 // Y coordinate (Northing-like)
}

// ConvertWGS84ToWCONGNAMUL converts coordinates from WGS84 to WCONGNAMUL.
// Implementation uses Transverse Mercator projection (Krüger n-series).
func ConvertWGS84ToWCONGNAMUL(lat, long float64) WCONGNAMULCoord {
	e, n := tmForward(lat*radiansPerDegree, long*radiansPerDegree)
	return WCONGNAMULCoord{
		X: math.Round(e * kakaoScale),
		Y: math.Round(n * kakaoScale),
	}
}

// ConvertWCONGToWGS84 translates WCONGNAMUL coordinates to WGS84.
func ConvertWCONGToWGS84(x, y float64) (float64, float64) {
	// Descale
	e := x / kakaoScale
	n := y / kakaoScale

	latRad, lonRad := tmInverse(e, n)
	return latRad * degreesPerRadian, lonRad * degreesPerRadian
}

// Ellipsoid parameters precomputed
var (
	n     = fWGS84 / (2.0 - fWGS84)
	alpha = []float64{
		1.0 / 2.0 * n,
		2.0 / 3.0 * n * n,
		5.0 / 16.0 * n * n * n,
		41.0 / 180.0 * n * n * n * n,
	}
	beta = []float64{
		1.0 / 2.0 * n,
		2.0 / 3.0 * n * n,
		37.0 / 96.0 * n * n * n,
		1.0 / 360.0 * n * n * n * n,
	}
	A0 = majorAxisWGS84 / (1.0 + n) * (1.0 + n*n/4.0 + n*n*n*n/64.0)
)

// tmForward converts (lat, lon) in radians to TM (E, N)
// Based on Krüger series expansion (order 4).
func tmForward(phi, lam float64) (float64, float64) {
	deltaLambda := lam - centerLon

	// Faster implementation using hyperbolic func approx:
	sinPhi := math.Sin(phi)
	cosPhi := math.Cos(phi)

	// Redfearn Implementation:
	// Constants for this phi:
	eSq := fWGS84 * (2 - fWGS84)
	nu := majorAxisWGS84 / math.Sqrt(1-eSq*sinPhi*sinPhi)
	rho := majorAxisWGS84 * (1 - eSq) / math.Pow(1-eSq*sinPhi*sinPhi, 1.5)
	eta2 := nu/rho - 1

	p := deltaLambda
	cos3Phi := cosPhi * cosPhi * cosPhi
	cos5Phi := cos3Phi * cosPhi * cosPhi
	tanPhi := math.Tan(phi)
	tan2Phi := tanPhi * tanPhi
	tan4Phi := tan2Phi * tan2Phi

	// Meridian Arc Length M (S in some texts)
	M := meridianArc(phi)

	// Easting
	// x = k0 * nu * [ p*cosPhi + (p^3/6)*cos^3Phi*(1 - t^2 + eta^2) + (p^5/120)*cos^5Phi*(5 - 18t^2 + t^4 + 14eta^2 - 58t^2eta^2) ]
	term1 := p * cosPhi
	term2 := math.Pow(p, 3) * cos3Phi * (1 - tan2Phi + eta2) / 6.0
	term3 := math.Pow(p, 5) * cos5Phi * (5 - 18*tan2Phi + tan4Phi + 14*eta2 - 58*tan2Phi*eta2) / 120.0

	E := falseEasting + scaleFactor*nu*(term1+term2+term3)

	// Northing
	// y = k0 * [ M - M0 + nu*tanPhi*( p^2/2 cos^2Phi + p^4/24 cos^4Phi(5 - t^2 + 9eta^2) + ... ) ]
	// M0 is M at centerLat.
	M0 := meridianArc(centerLat)

	tn1 := math.Pow(p, 2) * cosPhi * cosPhi / 2.0
	tn2 := math.Pow(p, 4) * math.Pow(cosPhi, 4) * (5 - tan2Phi + 9*eta2) / 24.0
	tn3 := math.Pow(p, 6) * math.Pow(cosPhi, 6) * (61 - 58*tan2Phi + tan4Phi) / 720.0

	N := falseNorthing + scaleFactor*((M-M0)+nu*tanPhi*(tn1+tn2+tn3))

	return E, N
}

func tmInverse(E, N float64) (float64, float64) {
	eSq := fWGS84 * (2 - fWGS84)
	e1 := (1 - math.Sqrt(1-eSq)) / (1 + math.Sqrt(1-eSq))

	M0 := meridianArc(centerLat)
	M := M0 + (N-falseNorthing)/scaleFactor

	// Calculate footprint latitude (mu)
	mu := M / (majorAxisWGS84 * (1 - eSq/4 - 3*eSq*eSq/64 - 5*math.Pow(eSq, 3)/256))

	// phi1 (Lat of footprint)
	// 3e1/2 - 27e1^3/32 ...
	c1 := (3*e1/2 - 27*math.Pow(e1, 3)/32)
	c2 := (21*e1*e1/16 - 55*math.Pow(e1, 4)/32)
	c3 := (151 * math.Pow(e1, 3) / 96)

	phi1 := mu + c1*math.Sin(2*mu) + c2*math.Sin(4*mu) + c3*math.Sin(6*mu)

	// Parameters at phi1
	sinPhi1 := math.Sin(phi1)
	cosPhi1 := math.Cos(phi1)
	tanPhi1 := math.Tan(phi1)

	factor := 1 - eSq*sinPhi1*sinPhi1
	nu1 := majorAxisWGS84 / math.Sqrt(factor)
	rho1 := majorAxisWGS84 * (1 - eSq) / math.Pow(factor, 1.5)
	eta1Sq := nu1/rho1 - 1

	D := (E - falseEasting) / (nu1 * scaleFactor)

	// Lat
	t1 := D * D / 2
	t2 := math.Pow(D, 4) / 24 * (5 + 3*tanPhi1*tanPhi1 + eta1Sq - 9*tanPhi1*tanPhi1*eta1Sq)
	t3 := math.Pow(D, 6) / 720 * (61 + 90*tanPhi1*tanPhi1 + 45*math.Pow(tanPhi1, 4))

	lat := phi1 - (nu1*tanPhi1/rho1)*(t1-t2+t3)

	// Lon
	l1 := D
	l2 := math.Pow(D, 3) / 6 * (1 + 2*tanPhi1*tanPhi1 + eta1Sq)
	l3 := math.Pow(D, 5) / 120 * (5 + 28*tanPhi1*tanPhi1 + 24*math.Pow(tanPhi1, 4) + 6*eta1Sq + 8*tanPhi1*tanPhi1*eta1Sq)

	lon := centerLon + (l1-l2+l3)/cosPhi1

	return lat, lon
}

// meridianArc calculates the arc length of the meridian from equator to phi
func meridianArc(phi float64) float64 {
	eSq := fWGS84 * (2 - fWGS84)

	// Coefficients for series
	// A = 1 + 3/4 e^2 + 45/64 e^4 + 175/256 e^6
	// B = 3/4 e^2 + 15/16 e^4 + 525/512 e^6
	// C = 15/64 e^4 + 105/256 e^6
	// D = 35/512 e^6

	A := 1 - eSq/4 - 3*eSq*eSq/64 - 5*math.Pow(eSq, 3)/256
	B := 3*eSq/8 + 3*eSq*eSq/32 + 45*math.Pow(eSq, 3)/1024
	C := 15*eSq*eSq/256 + 45*math.Pow(eSq, 3)/1024
	D := 35 * math.Pow(eSq, 3) / 3072

	return majorAxisWGS84 * (A*phi - B*math.Sin(2*phi) + C*math.Sin(4*phi) - D*math.Sin(6*phi))
}

// hasPrefixInRadix checks if any key in the radix tree starts with the term using WalkPrefix
func hasPrefixInRadix(tree *iradix.Tree[int], term string) bool {
	termBytes := []byte(term)
	found := false

	// Walk through the radix tree starting with the prefix
	tree.Root().WalkPrefix(termBytes, func(k []byte, v int) bool {
		found = true
		return false // Stop walking once we find the first match
	})

	return found
}

// Check if the term is a province or a prefix of any province (In South Korea)
func IsProvince(term string) bool {
	return hasPrefixInRadix(provinceRadix, term)
}

// Check if the term is a city or a prefix of any city (In South Korea)
func IsCity(term string) bool {
	return hasPrefixInRadix(cityRadix, term)
}

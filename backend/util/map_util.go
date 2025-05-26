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
	// Constants related to the WGS84 ellipsoid.
	aWGS84           float64 = 6378137 // Semi-major axis.
	flatteningFactor float64 = 0.0033528106647474805

	// Constants for Korea TM projection.
	k0               float64 = 1      // Scale factor.
	dx               float64 = 500000 // False Easting.
	dy               float64 = 200000 // False Northing.
	lat0             float64 = 38     // Latitude of origin.
	lon0             float64 = 127    // Longitude of origin.
	RadiansPerDegree float64 = math.Pi / 180
	RadiansToDegrees float64 = 180 / math.Pi
	scaleFactor      float64 = 2.5
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
	X float64 // X coordinate
	Y float64 // Y coordinate
}

// ConvertWGS84ToWCONGNAMUL converts coordinates from WGS84 to WCONGNAMUL.
func ConvertWGS84ToWCONGNAMUL(lat, long float64) WCONGNAMULCoord {
	x, y := transformWGS84ToKoreaTM(aWGS84, flatteningFactor, dx, dy, k0, lat0, lon0, lat, long)
	return WCONGNAMULCoord{X: math.Round(x * scaleFactor), Y: math.Round(y * scaleFactor)}
}

// transformWGS84ToKoreaTM optimizes the coordinate conversion calculation.
// func transformWGS84ToKoreaTM(aWGS84, flatteningFactor, dx, dy, k0, lat0, lon0, lat, long)
func transformWGS84ToKoreaTM(aWGS84, flatteningFactor, dx, dy, k0, lat0, lon0, lat, lon float64) (float64, float64) {
	latRad := lat * RadiansPerDegree
	lonRad := lon * RadiansPerDegree
	lRad := lat0 * RadiansPerDegree
	mRad := lon0 * RadiansPerDegree

	sinLat := math.Sin(latRad)
	cosLat := math.Cos(latRad)
	tanLat := sinLat / cosLat

	// Precompute powers of cosLat
	cosLat2 := cosLat * cosLat
	cosLat3 := cosLat2 * cosLat
	cosLat5 := cosLat3 * cosLat2
	cosLat7 := cosLat5 * cosLat2

	// Precompute repeated sines at multiple angles
	sin2Lat := math.Sin(2 * latRad)
	sin4Lat := math.Sin(4 * latRad)
	sin6Lat := math.Sin(6 * latRad)
	sin8Lat := math.Sin(8 * latRad)

	sin2L := math.Sin(2 * lRad)
	sin4L := math.Sin(4 * lRad)
	sin6L := math.Sin(6 * lRad)
	sin8L := math.Sin(8 * lRad)

	// Since flatteningFactor < 1 for WGS84, we can remove the condition
	w := 1 / flatteningFactor
	z := aWGS84 * (w - 1) / w
	dSquared := aWGS84 * aWGS84
	zSquared := z * z
	G := 1 - zSquared/dSquared
	w = (dSquared - zSquared) / zSquared

	// Simplify z calculation
	z = (aWGS84 - z) / (aWGS84 + z)
	z2 := z * z
	z3 := z2 * z
	z4 := z3 * z
	z5 := z4 * z

	E := aWGS84 * (1 - z + (5.0/4.0)*(z2-z3) + (81.0/64.0)*(z4-z5))
	I := (3.0 / 2.0) * aWGS84 * (z - z2 + (7.0/8.0)*(z3-z4) + (55.0/64.0)*z5)
	J := (15.0 / 16.0) * aWGS84 * (z2 - z3 + (3.0/4.0)*(z4-z5))
	L := (35.0 / 48.0) * aWGS84 * (z3 - z4 + (11.0/16.0)*z5)
	M := (315.0 / 512.0) * aWGS84 * (z4 - z5)

	D := lonRad - mRad

	// Compute u for lRad
	u_l := E*lRad - I*sin2L + J*sin4L - L*sin6L + M*sin8L
	z = u_l * k0

	// G recalculated for lat
	G = aWGS84 / math.Sqrt(1-G*sinLat*sinLat)

	// Compute u for latRad
	u_lat := E*latRad - I*sin2Lat + J*sin4Lat - L*sin6Lat + M*sin8Lat
	o := u_lat * k0

	// Compute polynomial expansions for easting (y)
	E_y := G * sinLat * cosLat * k0 * 0.5
	I_y := G * sinLat * cosLat3 * k0 * (5 - tanLat*tanLat + 9*w + 4*w*w) / 24
	J_y := G * sinLat * cosLat5 * k0 * (61 - 58*tanLat*tanLat + (tanLat*tanLat)*(tanLat*tanLat) +
		270*w - 330*tanLat*tanLat*w + 445*w*w + 324*w*w*w - 680*tanLat*tanLat*w*w + 88*w*w*w*w -
		600*tanLat*tanLat*w*w*w - 192*tanLat*tanLat*w*w*w*w) / 720
	H_y := G * sinLat * cosLat7 * k0 * (1385 - 3111*tanLat*tanLat + (tanLat*tanLat)*(tanLat*tanLat)*543 -
		(tanLat*tanLat)*(tanLat*tanLat)*(tanLat*tanLat)*tanLat) / 40320

	o += D*D*E_y + D*D*D*I_y + D*D*D*D*D*J_y + D*D*D*D*D*D*D*H_y
	y := o - z + dx

	// Compute polynomial expansions for northing (x)
	o_x := G * cosLat * k0
	z_x := G * cosLat3 * k0 * (1 - tanLat*tanLat + w) / 6
	w_x := G * cosLat5 * k0 * (5 - 18*tanLat*tanLat + (tanLat*tanLat)*(tanLat*tanLat) +
		14*w - 58*tanLat*tanLat*w + 13*w*w + 4*w*w*w -
		64*tanLat*tanLat*w*w - 25*tanLat*tanLat*w*w*w) / 120
	u_x := G * cosLat7 * k0 * (61 - 479*tanLat*tanLat + (tanLat*tanLat)*(tanLat*tanLat)*179 -
		(tanLat*tanLat)*(tanLat*tanLat)*(tanLat*tanLat)*tanLat) / 5040

	x := dy + D*o_x + D*D*D*z_x + D*D*D*D*D*w_x + D*D*D*D*D*D*D*u_x

	return x, y
}

// ConvertWCONGToWGS84 translates WCONGNAMUL coordinates to WGS84.
func ConvertWCONGToWGS84(x, y float64) (float64, float64) {
	return transformKoreaTMToWGS84(aWGS84, flatteningFactor, dx, dy, k0, lat0, lon0, x/2.5, y/2.5)
}

// transformKoreaTMToWGS84 transforms coordinates from Korea TM to WGS84.
func transformKoreaTMToWGS84(d, e, h, f, c, l, m, x, y float64) (float64, float64) {
	u := e
	if u > 1 {
		u = 1 / u
	}
	w := wConst // Conversion factor from degrees to radians
	o := l * w
	D := m * w
	u = 1 / u
	B := d * (u - 1) / u
	z := (d*d - B*B) / (d * d)
	u = (d*d - B*B) / (B * B)
	B = (d - B) / (d + B)

	G := d * (1 - B + 5*(B*B-B*B*B)/4 + 81*(B*B*B*B-B*B*B*B*B)/64)
	E := 3 * d * (B - B*B + 7*(B*B*B-B*B*B*B)/8 + 55*B*B*B*B*B/64) / 2
	I := 15 * d * (B*B - B*B*B + 3*(B*B*B*B-B*B*B*B*B)/4) / 16
	J := 35 * d * (B*B*B - B*B*B*B + 11*B*B*B*B*B/16) / 48
	L := 315 * d * (B*B*B*B - B*B*B*B*B) / 512

	o = G*o - E*math.Sin(2*o) + I*math.Sin(4*o) - J*math.Sin(6*o) + L*math.Sin(8*o)
	o *= c
	o = y + o - h
	M := o / c
	H := d * (1 - z) / math.Pow(math.Sqrt(1-z*math.Pow(math.Sin(0), 2)), 3)
	o = M / H
	for i := 0; i < 5; i++ {
		B = G*o - E*math.Sin(2*o) + I*math.Sin(4*o) - J*math.Sin(6*o) + L*math.Sin(8*o)
		H = d * (1 - z) / math.Pow(math.Sqrt(1-z*math.Pow(math.Sin(o), 2)), 3)
		o += (M - B) / H
	}
	H = d * (1 - z) / math.Pow(math.Sqrt(1-z*math.Pow(math.Sin(o), 2)), 3)
	G = d / math.Sqrt(1-z*math.Pow(math.Sin(o), 2))
	B = math.Sin(o)
	z = math.Cos(o)
	E = B / z
	u *= z * z
	A := x - f
	B = E / (2 * H * G * math.Pow(c, 2))
	I = E * (5 + 3*E*E + u - 4*u*u - 9*E*E*u) / (24 * H * G * G * G * math.Pow(c, 4))
	J = E * (61 + 90*E*E + 46*u + 45*E*E*E*E - 252*E*E*u - 3*u*u + 100*u*u*u - 66*E*E*u*u - 90*E*E*E*E*u + 88*u*u*u*u + 225*E*E*E*E*u*u + 84*E*E*u*u*u - 192*E*E*u*u*u*u) / (720 * H * G * G * G * G * G * math.Pow(c, 6))
	H = E * (1385 + 3633*E*E + 4095*E*E*E*E + 1575*E*E*E*E*E*E) / (40320 * H * G * G * G * G * G * G * G * math.Pow(c, 8))
	o = o - math.Pow(A, 2)*B + math.Pow(A, 4)*I - math.Pow(A, 6)*J + math.Pow(A, 8)*H
	B = 1 / (G * z * c)
	H = (1 + 2*E*E + u) / (6 * G * G * G * z * z * z * math.Pow(c, 3))
	u = (5 + 6*u + 28*E*E - 3*u*u + 8*E*E*u + 24*E*E*E*E - 4*u*u*u + 4*E*E*u*u + 24*E*E*u*u*u) / (120 * G * G * G * G * G * z * z * z * z * z * math.Pow(c, 5))
	z = (61 + 662*E*E + 1320*E*E*E*E + 720*E*E*E*E*E*E) / (5040 * G * G * G * G * G * G * G * z * z * z * z * z * z * z * math.Pow(c, 7))
	A = A*B - math.Pow(A, 3)*H + math.Pow(A, 5)*u - math.Pow(A, 7)*z
	D += A

	return o / w, D / w // LATITUDE, LONGITUDE
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

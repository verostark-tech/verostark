// cmd/gentest generates a synthetic CISAC CRD 3.0 R5 file with 100 works
// for stress-testing the Verostark detection pipeline.
//
// Usage:  go run ./cmd/gentest
// Output: testdata/verostark_stress_100_works.crd
package main

import (
	"fmt"
	"math/rand"
	"os"
	"strings"
)

// ── CRD fixed-width builders ──────────────────────────────────────────────────
// All positions are 1-indexed per CISAC CRD 3.0 R5.

func blank(n int) []byte {
	b := make([]byte, n)
	for i := range b {
		b[i] = ' '
	}
	return b
}

func setR(b []byte, pos, size int, val string) {
	idx := pos - 1
	for i := 0; i < size; i++ {
		if i < len(val) {
			b[idx+i] = val[i]
		}
		// else leave space
	}
}

func setN(b []byte, pos, size int, val int64) {
	setR(b, pos, size, fmt.Sprintf("%0*d", size, val))
}

// SDN: amount_decimals pos 113 size 1 | currency pos 120 size 3
func sdnLine() string {
	b := blank(122)
	copy(b[0:3], "SDN")
	b[112] = '2'
	copy(b[119:122], "SEK")
	return string(b)
}

// MWN: work_ref pos 20 size 14 | work_title pos 34 size 60 | iswc pos 154 size 11
func mwnLine(ref, title, iswc string) string {
	b := blank(164)
	copy(b[0:3], "MWN")
	setR(b, 20, 14, ref)
	setR(b, 34, 60, title)
	setR(b, 154, 11, iswc)
	return string(b)
}

// MDR: right_code pos 20 size 2 | right_category pos 22 size 3
func mdrLine() string {
	b := blank(24)
	copy(b[0:3], "MDR")
	copy(b[19:21], "MD")
	copy(b[21:24], "MEC")
	return string(b)
}

// MIP: numerator pos 36 size 5 | denominator pos 41 size 5
func mipLine(num, den int64) string {
	b := blank(45)
	copy(b[0:3], "MIP")
	copy(b[35:40], fmt.Sprintf("%05d", num))
	copy(b[40:45], fmt.Sprintf("%05d", den))
	return string(b)
}

// WER: dist_category pos 35 size 2 | currency pos 246 size 3
//      gross pos 249 size 18 | net pos 350 size 18
func werLine(gross, net int64) string {
	b := blank(367)
	copy(b[0:3], "WER")
	copy(b[34:36], "ME")
	copy(b[245:248], "SEK")
	copy(b[248:266], fmt.Sprintf("%018d", gross))
	copy(b[349:367], fmt.Sprintf("%018d", net))
	return string(b)
}

// ── Math helpers ──────────────────────────────────────────────────────────────

func gcd(a, b int64) int64 {
	if b == 0 {
		return a
	}
	return gcd(b, a%b)
}

// minGrossUnit returns the minimum gross that divides exactly for a clean payment.
// Requirement: gross × num divisible by 3 × den.
func minGrossUnit(num, den int64) int64 {
	td := 3 * den
	return td / gcd(num, td)
}

// cleanNet computes net = gross × num / (3 × den). Exact only when gross is a
// multiple of minGrossUnit(num,den).
func cleanNet(gross, num, den int64) int64 {
	return gross * num / (3 * den)
}

// devNet computes net for a target ratio_excess: net = gross × R × num / (3 × den).
func devNet(gross, num, den int64, ratioExcess float64) int64 {
	return int64(float64(gross) * ratioExcess * float64(num) / (3.0 * float64(den)))
}

// ── Work data ─────────────────────────────────────────────────────────────────

type event struct{ gross, net int64 }

type work struct {
	ref, title, iswc string
	num, den         int64
	events           []event
}

// Controlled share variants: 12 types covering 10%–100%
var shares = [][2]int64{
	{10000, 10000}, // 100 %
	{7500, 10000},  // 75 %
	{5000, 10000},  // 50 %
	{3333, 10000},  // 33.33 %
	{2500, 10000},  // 25 %
	{6667, 10000},  // 66.67 %
	{4000, 10000},  // 40 %
	{8000, 10000},  // 80 %
	{1000, 10000},  // 10 %
	{9000, 10000},  // 90 %
	{1500, 10000},  // 15 %
	{6000, 10000},  // 60 %
}

// 100 distinct titles: Swedish, English, musical, hybrid
var allTitles = [100]string{
	// 1-25: Swedish
	"SOMMARNATT", "DROMMAR", "HJARTATS SANG", "HAVET KALLAR", "VINTERMELODI",
	"REGN OCH SOL", "STORMEN", "LJUSET ATERGAR", "MORKETS ROST", "KARLEKENS VAG",
	"FRIHETENS VIND", "HIMLENS PORT", "STJARNORNAS DANS", "MANENS SKUGGA", "BERGETS EKKO",
	"SKOGENS HEMLIGHET", "FLODENS SANG", "STADENS PULS", "HEMVAGEN", "HOPPETS FLAMMA",
	"SOLUPPGANG", "SKYMNING", "GRYNING", "MIDNATT OVER STOCKHOLM", "KVALLSANG",
	// 26-50: English
	"BLUE HORIZON", "OCEAN DRIVE", "MIDNIGHT EXPRESS", "GOLDEN HOUR", "SILVER RAIN",
	"NORTHERN LIGHTS", "ARCTIC DREAM", "BOREAL TIDE", "TUNDRA SONG", "FJORD ECHO",
	"BLACK FOREST WALK", "WHITE MOUNTAINS HIGH", "RED AURORA RISING", "GREEN VALLEY LOW", "DEEP BLUE SKY",
	"EARLY MORNING FOG", "LATE EVENING GLOW", "SUNDAY SILENCE FALLS", "FRIDAY FEELING GOOD", "WEEKEND MOOD SHIFT",
	"SPRING RAIN FALLS SOFT", "SUMMER HEAT WAVE LONG", "AUTUMN LEAVES TURN RED", "WINTER FROST BITES HARD", "HARVEST MOON RISES SLOW",
	// 51-75: Musical terms
	"BALLAD IN D MINOR", "NOCTURNE NUMBER SEVEN", "PRELUDE IN C MINOR", "ETUDE FOR STRINGS", "FANTASIA BOREALIS",
	"VALS OVER DALARNA", "POLKA STOCKHOLM STYLE", "SERENAD TILL SOFIA", "CAPRICCIO IN G MAJOR", "INTERMEZZO LENTO",
	"DIGITAL DAWN BREAKS", "ANALOG DUSK SETTLES", "SYNTHETIC SOUL RISES", "ELECTRIC DREAM FADES", "NEON TIDE FLOWS",
	"QUANTUM LEAP FORWARD", "NEURAL DRIFT SIDEWAYS", "BINARY BEAT DROPS", "PIXEL PULSE QUICKENS", "VECTOR GROOVE LOCKS",
	"ISLAND BREEZE CALLS SOFT", "COASTAL CALM WATERS STILL", "HIGHLAND MIST ROLLS THICK", "RIVER DEEP RUNS STRONG", "DESERT WIND BLOWS HARD",
	// 76-100: Hybrid / geographic
	"STOCKHOLM BLUES SLOW", "GOTHENBURG GROOVE FAST", "MALMO SHUFFLE DANCE NOW", "GAMLA STAN WALTZ OLD", "SODERMALM NIGHTS DARK",
	"NORRLAND DREAM FADES FAR", "DALARNA DANCE STEPS LIGHT", "SMALAND STOMP HEAVY BEAT", "VASTERAS VIBE CHECK QUICK", "OREBRO NIGHT LIGHTS BURN",
	"VAGSANG PA HAVET WIDE", "BARNVISA TILL NATTEN DEEP", "FOLKVISA FRAN NORRLAND HIGH", "DANSMELODI SOMMAR BRIGHT", "MARSCHTAKT STOCKHOLM BOLD",
	"SNOWFALL WALTZ QUICK PACE", "THUNDERSTORM BOOGIE WILD", "HEATWAVE SHUFFLE JAZZ HOT", "RAINBOW CHASE SUNSET FAR", "FOGHORN BLUES DRIFT SLOW",
	"ALTA VISTA DREAMS BRIGHT", "TERRA NOVA RISING HIGH", "AQUA MARE FLOWING DEEP", "IGNIS FATUUS BURNS LOW", "VENTUS LEVIS WHISPERS SOFT",
}

func iswc(i int) string {
	// T + 9 digits + single check digit — parser reads raw string, no validation
	n := 1000000 + i
	check := i % 10
	return fmt.Sprintf("T%09d%d", n, check)
}

func ref(i int) string {
	return fmt.Sprintf("STIM%08d", 10000000+i)
}

// ── Work builder ──────────────────────────────────────────────────────────────

func buildWorks(rng *rand.Rand) []work {
	works := make([]work, 0, 100)
	idx := 0 // running title/ref index

	newWork := func(num, den int64, evs []event) work {
		w := work{
			ref:    ref(idx + 1),
			title:  allTitles[idx],
			iswc:   iswc(idx + 1),
			num:    num,
			den:    den,
			events: evs,
		}
		idx++
		return w
	}

	// ── Group 1: CLEAN (25 works) — exact rational payment ───────────────────
	// Each (gross, num, den) satisfies: gross × num ≡ 0 (mod 3 × den)
	cleanCases := [][3]int64{
		// gross, num, den
		{300000, 10000, 10000},   //  3 000 SEK | 100 %
		{1500000, 10000, 10000},  // 15 000 SEK | 100 %
		{6000000, 10000, 10000},  // 60 000 SEK | 100 %
		{90000, 10000, 10000},    //    900 SEK | 100 %
		{9000000, 10000, 10000},  // 90 000 SEK | 100 %
		{400000, 7500, 10000},    //  4 000 SEK |  75 %
		{1000000, 7500, 10000},   // 10 000 SEK |  75 %
		{150000, 5000, 10000},    //  1 500 SEK |  50 %
		{6000000, 5000, 10000},   // 60 000 SEK |  50 %
		{100000, 3333, 10000},    //  1 000 SEK |  33 %
		{1000000, 3333, 10000},   // 10 000 SEK |  33 %
		{5000000, 3333, 10000},   // 50 000 SEK |  33 %
		{360000, 2500, 10000},    //  3 600 SEK |  25 %
		{1200000, 2500, 10000},   // 12 000 SEK |  25 %
		{300000, 6667, 10000},    //  3 000 SEK |  67 %
		{3000000, 6667, 10000},   // 30 000 SEK |  67 %
		{750000, 4000, 10000},    //  7 500 SEK |  40 %
		{150000, 4000, 10000},    //  1 500 SEK |  40 %
		{375000, 8000, 10000},    //  3 750 SEK |  80 %
		{3000000, 1000, 10000},   // 30 000 SEK |  10 %
		{9000000, 1000, 10000},   // 90 000 SEK |  10 %
		{1000000, 9000, 10000},   // 10 000 SEK |  90 %
		{1000000, 1500, 10000},   // 10 000 SEK |  15 %
		{500000, 6000, 10000},    //  5 000 SEK |  60 %
		{2500000, 6000, 10000},   // 25 000 SEK |  60 %
	}
	for _, c := range cleanCases {
		gross, num, den := c[0], c[1], c[2]
		net := cleanNet(gross, num, den)
		works = append(works, newWork(num, den, []event{{gross, net}}))
	}

	// ── Group 2: POSSIBLE overpayment (15 works, ratio_excess 1.001–1.099) ──
	possibleRatios := []float64{
		1.002, 1.008, 1.015, 1.022, 1.030,
		1.041, 1.053, 1.062, 1.071, 1.082,
		1.088, 1.092, 1.003, 1.045, 1.097,
	}
	possibleShares := [][2]int64{
		{10000, 10000}, {7500, 10000}, {5000, 10000}, {3333, 10000}, {2500, 10000},
		{6667, 10000}, {4000, 10000}, {8000, 10000}, {1000, 10000}, {9000, 10000},
		{1500, 10000}, {6000, 10000}, {10000, 10000}, {5000, 10000}, {7500, 10000},
	}
	possibleGross := []int64{
		372000, 500000, 180000, 2100000, 750000,
		3600000, 280000, 900000, 6000000, 450000,
		1500000, 800000, 1200000, 4500000, 2700000,
	}
	for i := 0; i < 15; i++ {
		num, den := possibleShares[i][0], possibleShares[i][1]
		gross := possibleGross[i]
		net := devNet(gross, num, den, possibleRatios[i])
		// 30% chance of a second income line with a different gross
		var evs []event
		evs = append(evs, event{gross, net})
		if rng.Float64() < 0.30 {
			g2 := gross / 2
			n2 := devNet(g2, num, den, possibleRatios[i]*0.99)
			evs = append(evs, event{g2, n2})
		}
		works = append(works, newWork(num, den, evs))
	}

	// ── Group 3: MEDIUM overpayment (20 works, ratio_excess 1.1–1.499) ──────
	mediumRatios := []float64{
		1.10, 1.13, 1.18, 1.22, 1.27,
		1.31, 1.35, 1.39, 1.43, 1.46,
		1.11, 1.15, 1.20, 1.25, 1.30,
		1.33, 1.37, 1.41, 1.45, 1.48,
	}
	for i := 0; i < 20; i++ {
		s := shares[i%len(shares)]
		num, den := s[0], s[1]
		gross := []int64{
			102600, 250000, 600000, 1400000, 3500000,
			85000, 420000, 980000, 2200000, 7500000,
			170000, 350000, 750000, 1800000, 4200000,
			65000, 320000, 870000, 2600000, 9000000,
		}[i]
		net := devNet(gross, num, den, mediumRatios[i])
		var evs []event
		evs = append(evs, event{gross, net})
		if rng.Float64() < 0.40 {
			g2 := int64(float64(gross) * (0.6 + rng.Float64()*0.8))
			n2 := devNet(g2, num, den, mediumRatios[i])
			evs = append(evs, event{g2, n2})
		}
		works = append(works, newWork(num, den, evs))
	}

	// ── Group 4: HIGH overpayment (20 works, ratio_excess 1.5–2.499) ────────
	highRatios := []float64{
		1.50, 1.58, 1.67, 1.75, 1.83,
		1.92, 2.00, 2.10, 2.20, 2.30,
		1.53, 1.62, 1.71, 1.80, 1.90,
		2.05, 2.15, 2.25, 2.35, 2.45,
	}
	for i := 0; i < 20; i++ {
		s := shares[(i+3)%len(shares)]
		num, den := s[0], s[1]
		gross := []int64{
			500000, 800000, 1200000, 2800000, 6000000,
			380000, 720000, 1600000, 3200000, 8500000,
			290000, 650000, 1100000, 2400000, 5500000,
			430000, 960000, 1900000, 4100000, 11000000,
		}[i]
		net := devNet(gross, num, den, highRatios[i])
		var evs []event
		evs = append(evs, event{gross, net})
		if rng.Float64() < 0.35 {
			g2 := int64(float64(gross) * (0.4 + rng.Float64()*1.2))
			n2 := devNet(g2, num, den, highRatios[i])
			evs = append(evs, event{g2, n2})
		}
		works = append(works, newWork(num, den, evs))
	}

	// ── Group 5: CRITICAL overpayment (15 works, ratio_excess 2.5–6.0) ──────
	criticalRatios := []float64{
		2.50, 2.70, 2.90, 3.00, 3.20,
		3.50, 3.80, 4.00, 4.30, 4.70,
		5.00, 5.30, 5.60, 5.80, 6.00,
	}
	for i := 0; i < 15; i++ {
		s := shares[(i+7)%len(shares)]
		num, den := s[0], s[1]
		gross := []int64{
			102600, 372000, 900000, 2100000, 5000000,
			750000, 1800000, 4200000, 9500000, 22000000,
			450000, 1100000, 2800000, 7000000, 18000000,
		}[i]
		net := devNet(gross, num, den, criticalRatios[i])
		var evs []event
		evs = append(evs, event{gross, net})
		if rng.Float64() < 0.50 {
			g2 := int64(float64(gross) * (0.3 + rng.Float64()))
			n2 := devNet(g2, num, den, criticalRatios[i])
			evs = append(evs, event{g2, n2})
		}
		works = append(works, newWork(num, den, evs))
	}

	// ── Group 6: Underpayments (5 works, ratio_excess 0.1–0.9) ──────────────
	underRatios := []float64{0.10, 0.33, 0.50, 0.75, 0.90}
	underGross := []int64{500000, 1200000, 3000000, 750000, 2100000}
	for i := 0; i < 5; i++ {
		s := shares[(i+5)%len(shares)]
		num, den := s[0], s[1]
		gross := underGross[i]
		net := devNet(gross, num, den, underRatios[i])
		works = append(works, newWork(num, den, []event{{gross, net}}))
	}

	return works
}

// ── Main ──────────────────────────────────────────────────────────────────────

func main() {
	rng := rand.New(rand.NewSource(42))
	works := buildWorks(rng)

	var sb strings.Builder
	sb.WriteString(sdnLine())
	sb.WriteByte('\n')

	for _, w := range works {
		sb.WriteString(mwnLine(w.ref, w.title, w.iswc))
		sb.WriteByte('\n')
		sb.WriteString(mdrLine())
		sb.WriteByte('\n')
		sb.WriteString(mipLine(w.num, w.den))
		sb.WriteByte('\n')
		for _, e := range w.events {
			sb.WriteString(werLine(e.gross, e.net))
			sb.WriteByte('\n')
		}
	}

	content := sb.String()

	if err := os.MkdirAll("testdata", 0755); err != nil {
		fmt.Fprintf(os.Stderr, "mkdir: %v\n", err)
		os.Exit(1)
	}
	path := "testdata/verostark_stress_100_works.crd"
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		fmt.Fprintf(os.Stderr, "write: %v\n", err)
		os.Exit(1)
	}

	werCount := 0
	for _, w := range works {
		werCount += len(w.events)
	}
	fmt.Printf("Generated %s\n", path)
	fmt.Printf("  Works:      %d\n", len(works))
	fmt.Printf("  WER lines:  %d\n", werCount)
	fmt.Printf("  CLEAN:      25  (works 1-25)\n")
	fmt.Printf("  POSSIBLE:   15  (works 26-40)\n")
	fmt.Printf("  MEDIUM:     20  (works 41-60)\n")
	fmt.Printf("  HIGH:       20  (works 61-80)\n")
	fmt.Printf("  CRITICAL:   15  (works 81-95)\n")
	fmt.Printf("  UNDERPAY:    5  (works 96-100)\n")
}

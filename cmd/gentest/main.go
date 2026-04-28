// cmd/gentest generates synthetic CISAC CRD 3.0 R5 files for testing.
//
// Usage:  go run ./cmd/gentest
// Output: testdata/verostark_*.crd  (4 files)
package main

import (
	"fmt"
	"math/rand"
	"os"
	"strings"
)

// ── CRD fixed-width builders ──────────────────────────────────────────────────

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
	}
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
//
//	gross pos 249 size 18 | net pos 350 size 18
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

func minGrossUnit(num, den int64) int64 {
	td := 3 * den
	return td / gcd(num, td)
}

func cleanNet(gross, num, den int64) int64 {
	return gross * num / (3 * den)
}

func devNet(gross, num, den int64, ratioExcess float64) int64 {
	return int64(float64(gross) * ratioExcess * float64(num) / (3.0 * float64(den)))
}

// ── Title pool (200 titles) ───────────────────────────────────────────────────

var titlePool = []string{
	// Swedish / Nordic
	"SOMMARNATT", "DROMMAR", "HJARTATS SANG", "HAVET KALLAR", "VINTERMELODI",
	"REGN OCH SOL", "STORMEN", "LJUSET ATERGAR", "MORKETS ROST", "KARLEKENS VAG",
	"FRIHETENS VIND", "HIMLENS PORT", "STJARNORNAS DANS", "MANENS SKUGGA", "BERGETS EKKO",
	"SKOGENS HEMLIGHET", "FLODENS SANG", "STADENS PULS", "HEMVAGEN", "HOPPETS FLAMMA",
	"SOLUPPGANG", "SKYMNING", "GRYNING", "MIDNATT OVER STOCKHOLM", "KVALLSANG",
	"VAGSANG PA HAVET", "BARNVISA TILL NATTEN", "FOLKVISA FRAN NORRLAND", "DANSMELODI SOMMAR", "MARSCHTAKT STOCKHOLM",
	"NORRLAND DREAM FADES", "DALARNA DANCE STEPS", "SMALAND STOMP HEAVY", "VASTERAS VIBE CHECK", "OREBRO NIGHT LIGHTS",
	"STOCKHOLM BLUES SLOW", "GOTHENBURG GROOVE FAST", "MALMO SHUFFLE DANCE", "GAMLA STAN WALTZ", "SODERMALM NIGHTS DARK",
	"VALS OVER DALARNA", "POLKA STOCKHOLM STYLE", "SERENAD TILL SOFIA", "CAPRICCIO IN G", "INTERMEZZO LENTO",
	"LJUSNARSBERGS VISA", "ANGLARNAS DANS", "TROLLETS SANG", "HAVSVINDENS ROSTA", "MIDVINTERBLOT",
	// English
	"BLUE HORIZON", "OCEAN DRIVE", "MIDNIGHT EXPRESS", "GOLDEN HOUR", "SILVER RAIN",
	"NORTHERN LIGHTS", "ARCTIC DREAM", "BOREAL TIDE", "TUNDRA SONG", "FJORD ECHO",
	"BLACK FOREST WALK", "WHITE MOUNTAINS HIGH", "RED AURORA RISING", "GREEN VALLEY LOW", "DEEP BLUE SKY",
	"EARLY MORNING FOG", "LATE EVENING GLOW", "SUNDAY SILENCE FALLS", "FRIDAY FEELING GOOD", "WEEKEND MOOD SHIFT",
	"SPRING RAIN FALLS SOFT", "SUMMER HEAT WAVE LONG", "AUTUMN LEAVES TURN RED", "WINTER FROST BITES HARD", "HARVEST MOON RISES SLOW",
	"ISLAND BREEZE CALLS SOFT", "COASTAL CALM WATERS STILL", "HIGHLAND MIST ROLLS THICK", "RIVER DEEP RUNS STRONG", "DESERT WIND BLOWS HARD",
	"SNOWFALL WALTZ QUICK PACE", "THUNDERSTORM BOOGIE WILD", "HEATWAVE SHUFFLE JAZZ HOT", "RAINBOW CHASE SUNSET FAR", "FOGHORN BLUES DRIFT SLOW",
	"ALTA VISTA DREAMS BRIGHT", "TERRA NOVA RISING HIGH", "AQUA MARE FLOWING DEEP", "IGNIS FATUUS BURNS LOW", "VENTUS LEVIS WHISPERS SOFT",
	"LOST IN THE STATIC", "RUNNING THROUGH MIDNIGHT", "PAPER PLANES AND STORMS", "GHOST IN THE SIGNAL", "SILVER TONGUE DEVIL",
	"COLD NOVEMBER MORNING", "WARM JULY EVENING", "BROKEN COMPASS NORTH", "FADED PHOTOGRAPH OLD", "EMPTY ROOM ECHOES",
	// Musical / classical
	"BALLAD IN D MINOR", "NOCTURNE NUMBER SEVEN", "PRELUDE IN C MINOR", "ETUDE FOR STRINGS", "FANTASIA BOREALIS",
	"DIGITAL DAWN BREAKS", "ANALOG DUSK SETTLES", "SYNTHETIC SOUL RISES", "ELECTRIC DREAM FADES", "NEON TIDE FLOWS",
	"QUANTUM LEAP FORWARD", "NEURAL DRIFT SIDEWAYS", "BINARY BEAT DROPS", "PIXEL PULSE QUICKENS", "VECTOR GROOVE LOCKS",
	"OPUS THREE IN F", "SONATA FOR TWO", "REQUIEM IN BLUE", "CONCERTO GROSSO", "FUGUE IN WINTER",
	"THEME FOR ANNA", "VARIATIONS ON A DREAM", "RONDO IN THE RAIN", "MINUET FOR CLARA", "TOCCATA IN AMBER",
	"SCHERZO FOR WIND", "LARGO FOR STRINGS", "ANDANTE IN GREY", "ALLEGRETTO IN GOLD", "PRESTO IN BLACK",
	"SUITE FOR THE LOST", "MARCH OF THE COAST", "DANCE OF THE BIRCH", "SONG OF THE IRON", "HYMN OF THE NORTH",
	"ELEGY FOR SPRING", "ODE TO THE FJORD", "ANTHEM OF STONE", "CAROL OF THE PINES", "DIRGE FOR THE SEA",
	"MAZURKA IN STORM", "BOLERO OF THE NIGHT", "TARANTELLA IN FLAME", "HABANERA IN FROST", "GIGUE IN THUNDER",
	"SARABANDE IN MIST", "COURANTE IN RAIN", "ALLEMANDE IN SNOW", "PAVANE IN ASH", "GALLIARD IN SMOKE",
	// Geographic / atmospheric
	"PACIFIC RIM SUNRISE", "ATLANTIC CROSSING COLD", "INDIAN SUMMER FADE", "ARCTIC CIRCLE NORTH", "EQUATOR LINE HEAT",
	"TROPICS IN WINTER", "POLAR LIGHT DANCE", "MONSOON SEASON LATE", "TRADE WIND STEADY", "MISTRAL BLOW HARD",
	"BORA BORA DRIFT", "SIROCCO SAND RUSH", "TRAMONTANE HIGH COLD", "LEVANTE EAST WARM", "PONENTE WEST CALM",
	"FOEHN VALLEY DOWN", "CHINOOK RANGE MELT", "ZEPHYR COAST LIGHT", "ETESIAN BLUE CLEAR", "HARMATTAN DRY DUST",
	"CAPE HORN FURY", "DRAKE PASSAGE WILD", "TIERRA DEL FUEGO END", "PATAGONIA WIND STRIP", "TIERRA ALTA THIN",
	"MIDNIGHT SUN NORTH", "POLAR NIGHT SOUTH", "EQUINOX BALANCE POINT", "SOLSTICE PEAK LIGHT", "PERIHELION CLOSE WARM",
	"APHELION FAR COLD", "ZENITH OVERHEAD HIGH", "NADIR BELOW DEEP", "HORIZON LINE FLAT", "VANISHING POINT FAR",
	"LONGITUDE ZERO PRIME", "LATITUDE SIXTY NORTH", "ALTITUDE MAXIMUM HIGH", "DEPTH MINIMUM LOW", "BEARING TRUE NORTH",
	// Modern / pop
	"THREE AM THOUGHTS", "SIGNAL AND NOISE", "VOLTAGE IN THE DARK", "PROTOCOL FOR RAIN", "ALGORITHM BLUES",
	"FEEDBACK LOOP OPEN", "CARRIER WAVE LOST", "BANDWIDTH EXCEEDED NOW", "LATENCY IN LOVE", "PACKET LOSS GRIEF",
	"INTERFACE BROKEN HEART", "BUFFER OVERFLOW TEARS", "SYNTAX ERROR SONG", "RUNTIME EXCEPTION JOY", "NULL POINTER DANCE",
	"STACK OVERFLOW WALTZ", "MEMORY LEAK LULLABY", "DEADLOCK TANGO SLOW", "RACE CONDITION QUICK", "SEMAPHORE SWING",
	"MUTEX MIDNIGHT SLOW", "THREAD POOL SHIMMY", "GARBAGE COLLECT BLUES", "COMPILE TIME BALLAD", "LINK ERROR HYMN",
}

// ── Work types ────────────────────────────────────────────────────────────────

type event struct{ gross, net int64 }

type work struct {
	ref, title, iswc string
	num, den         int64
	events           []event
}

var shares = [][2]int64{
	{10000, 10000}, {7500, 10000}, {5000, 10000}, {3333, 10000},
	{2500, 10000}, {6667, 10000}, {4000, 10000}, {8000, 10000},
	{1000, 10000}, {9000, 10000}, {1500, 10000}, {6000, 10000},
}

// 50 pre-computed clean (gross, num, den) combos.
// All satisfy: gross × num ≡ 0 (mod 3 × den).
var cleanCases = [][3]int64{
	{300000, 10000, 10000}, {1500000, 10000, 10000}, {6000000, 10000, 10000}, {90000, 10000, 10000}, {9000000, 10000, 10000},
	{400000, 7500, 10000}, {1000000, 7500, 10000}, {4000000, 7500, 10000}, {100000, 7500, 10000}, {200000, 7500, 10000},
	{150000, 5000, 10000}, {6000000, 5000, 10000}, {300000, 5000, 10000}, {900000, 5000, 10000}, {3000000, 5000, 10000},
	{100000, 3333, 10000}, {1000000, 3333, 10000}, {5000000, 3333, 10000}, {500000, 3333, 10000}, {200000, 3333, 10000},
	{360000, 2500, 10000}, {1200000, 2500, 10000}, {120000, 2500, 10000}, {3600000, 2500, 10000}, {600000, 2500, 10000},
	{300000, 6667, 10000}, {3000000, 6667, 10000}, {600000, 6667, 10000}, {6000000, 6667, 10000}, {150000, 6667, 10000},
	{750000, 4000, 10000}, {150000, 4000, 10000}, {3000000, 4000, 10000}, {450000, 4000, 10000}, {1500000, 4000, 10000},
	{375000, 8000, 10000}, {3000000, 8000, 10000}, {750000, 8000, 10000}, {150000, 8000, 10000}, {1500000, 8000, 10000},
	{3000000, 1000, 10000}, {9000000, 1000, 10000}, {300000, 1000, 10000}, {6000000, 1000, 10000}, {1500000, 1000, 10000},
	{1000000, 9000, 10000}, {5000000, 9000, 10000}, {200000, 9000, 10000}, {1000000, 1500, 10000}, {500000, 6000, 10000},
}

// ── File config ───────────────────────────────────────────────────────────────

type fileConfig struct {
	filename  string
	seed      int64
	refPrefix string
	clean     int
	possible  int
	medium    int
	high      int
	critical  int
	underpay  int
}

func totalWorks(cfg fileConfig) int {
	return cfg.clean + cfg.possible + cfg.medium + cfg.high + cfg.critical + cfg.underpay
}

// ── Work builder ──────────────────────────────────────────────────────────────

func buildWorks(cfg fileConfig) []work {
	rng := rand.New(rand.NewSource(cfg.seed))
	n := totalWorks(cfg)

	// Shuffle title indices so each file has a distinct ordering.
	poolSize := len(titlePool)
	titleIdx := make([]int, poolSize)
	for i := range titleIdx {
		titleIdx[i] = i
	}
	rng.Shuffle(poolSize, func(i, j int) { titleIdx[i], titleIdx[j] = titleIdx[j], titleIdx[i] })

	works := make([]work, 0, n)
	seq := 0 // sequential index within file

	newWork := func(num, den int64, evs []event) work {
		w := work{
			ref:    fmt.Sprintf("%s%06d", cfg.refPrefix, 100000+seq+1),
			title:  titlePool[titleIdx[seq%poolSize]],
			iswc:   fmt.Sprintf("T%09d%d", 1000000+seq+1, (seq+1)%10),
			num:    num,
			den:    den,
			events: evs,
		}
		seq++
		return w
	}

	// ── CLEAN ─────────────────────────────────────────────────────────────────
	for i := 0; i < cfg.clean; i++ {
		c := cleanCases[i%len(cleanCases)]
		gross, num, den := c[0], c[1], c[2]
		// Scale gross randomly so amounts are varied across files.
		scales := []int64{1, 2, 3, 5, 7, 10, 15, 20}
		gross *= scales[rng.Intn(len(scales))]
		net := cleanNet(gross, num, den)
		works = append(works, newWork(num, den, []event{{gross, net}}))
	}

	// ── POSSIBLE (ratio_excess 1.001–1.099) ───────────────────────────────────
	for i := 0; i < cfg.possible; i++ {
		s := shares[rng.Intn(len(shares))]
		num, den := s[0], s[1]
		gross := grossPool[rng.Intn(len(grossPool))]
		ratio := 1.001 + rng.Float64()*0.098
		net := devNet(gross, num, den, ratio)
		evs := []event{{gross, net}}
		if rng.Float64() < 0.30 {
			g2 := gross / 2
			evs = append(evs, event{g2, devNet(g2, num, den, ratio)})
		}
		works = append(works, newWork(num, den, evs))
	}

	// ── MEDIUM (ratio_excess 1.1–1.499) ──────────────────────────────────────
	for i := 0; i < cfg.medium; i++ {
		s := shares[rng.Intn(len(shares))]
		num, den := s[0], s[1]
		gross := grossPool[rng.Intn(len(grossPool))]
		ratio := 1.1 + rng.Float64()*0.399
		net := devNet(gross, num, den, ratio)
		evs := []event{{gross, net}}
		if rng.Float64() < 0.40 {
			g2 := int64(float64(gross) * (0.5 + rng.Float64()))
			evs = append(evs, event{g2, devNet(g2, num, den, ratio)})
		}
		works = append(works, newWork(num, den, evs))
	}

	// ── HIGH (ratio_excess 1.5–2.499) ────────────────────────────────────────
	for i := 0; i < cfg.high; i++ {
		s := shares[rng.Intn(len(shares))]
		num, den := s[0], s[1]
		gross := grossPool[rng.Intn(len(grossPool))]
		ratio := 1.5 + rng.Float64()*0.999
		net := devNet(gross, num, den, ratio)
		evs := []event{{gross, net}}
		if rng.Float64() < 0.35 {
			g2 := int64(float64(gross) * (0.4 + rng.Float64()*1.2))
			evs = append(evs, event{g2, devNet(g2, num, den, ratio)})
		}
		works = append(works, newWork(num, den, evs))
	}

	// ── CRITICAL (ratio_excess ≥ 2.5) ────────────────────────────────────────
	for i := 0; i < cfg.critical; i++ {
		s := shares[rng.Intn(len(shares))]
		num, den := s[0], s[1]
		gross := grossPool[rng.Intn(len(grossPool))]
		ratio := 2.5 + rng.Float64()*3.5
		net := devNet(gross, num, den, ratio)
		evs := []event{{gross, net}}
		if rng.Float64() < 0.50 {
			g2 := int64(float64(gross) * (0.3 + rng.Float64()))
			evs = append(evs, event{g2, devNet(g2, num, den, ratio)})
		}
		works = append(works, newWork(num, den, evs))
	}

	// ── UNDERPAYMENT (ratio_excess 0.05–0.95, flagged) ───────────────────────
	for i := 0; i < cfg.underpay; i++ {
		s := shares[rng.Intn(len(shares))]
		num, den := s[0], s[1]
		gross := grossPool[rng.Intn(len(grossPool))]
		ratio := 0.05 + rng.Float64()*0.90
		net := devNet(gross, num, den, ratio)
		evs := []event{{gross, net}}
		if rng.Float64() < 0.40 {
			g2 := int64(float64(gross) * (0.5 + rng.Float64()*0.8))
			evs = append(evs, event{g2, devNet(g2, num, den, ratio)})
		}
		works = append(works, newWork(num, den, evs))
	}

	return works
}

// grossPool covers 500 SEK to 220,000 SEK across several orders of magnitude.
var grossPool = []int64{
	50000, 75000, 100000, 150000, 200000, 280000, 372000, 500000,
	650000, 900000, 1200000, 1600000, 2100000, 2800000, 3500000,
	4500000, 5500000, 7000000, 9000000, 11000000, 15000000, 22000000,
}

// ── Edge cases file ───────────────────────────────────────────────────────────

// sdnLineWithPeriod builds an SDN record that includes period_start and
// period_end so the auto-period extraction can be tested.
// period_start: pos 33 size 8  period_end: pos 41 size 8
func sdnLineWithPeriod(start, end string) string {
	b := blank(122)
	copy(b[0:3], "SDN")
	setR(b, 33, 8, start)
	setR(b, 41, 8, end)
	b[112] = '2'
	copy(b[119:122], "SEK")
	return string(b)
}

// rawRecord produces a minimal record of the given 3-char type padded to 100
// chars. Used for ADJ, FEO, ICC records that must be silently skipped.
func rawRecord(recType string) string {
	b := blank(100)
	copy(b[0:3], recType)
	return string(b)
}

// writeEdgeCases generates verostark_edge_cases.crd.
// Five works, each targeting one parser edge case:
//
//	Work 1 – Swedish characters (å, ö, ä)         → CLEAN
//	Work 2 – Space-padded optional WER fields      → CLEAN
//	ADJ    – Adjustment record between MWN blocks  → SKIPPED
//	Work 3 – First work after ADJ                  → CRITICAL
//	FEO    – Fees-in-Error record between blocks   → SKIPPED
//	Work 4 – First work after FEO                  → CLEAN
//	Work 5 – ICC record inside MWN block           → CRITICAL (ICC ignored)
func writeEdgeCases() {
	var sb strings.Builder

	sb.WriteString(sdnLineWithPeriod("20250101", "20250331") + "\n")

	// ── Work 1: Swedish characters, CLEAN ─────────────────────────────────────
	// Å = U+00C5, Ö = U+00D6, Ä = U+00C4 — all 2-byte UTF-8.
	// gross=300000 cents (3000.00 SEK)  net=100000 cents (1000.00 SEK)
	// ratio = 100000/300000 = 1/3 = expected → CLEAN
	sb.WriteString(mwnLine("STIM20250001", "ÅSKAN ÖVER FJÄLLEN", "T2000000010") + "\n")
	sb.WriteString(mdrLine() + "\n")
	sb.WriteString(mipLine(10000, 10000) + "\n")
	sb.WriteString(werLine(300000, 100000) + "\n")

	// ── Work 2: Space-padded optional WER fields, CLEAN ───────────────────────
	// werLine() already fills every non-required field with spaces.
	// This asserts the parser handles space-only optional fields without error.
	sb.WriteString(mwnLine("STIM20250002", "EMPTY FIELDS TEST", "T2000000020") + "\n")
	sb.WriteString(mdrLine() + "\n")
	sb.WriteString(mipLine(10000, 10000) + "\n")
	sb.WriteString(werLine(300000, 100000) + "\n")

	// ── ADJ: silently skipped ──────────────────────────────────────────────────
	sb.WriteString(rawRecord("ADJ") + "\n")

	// ── Work 3: After ADJ, CRITICAL ───────────────────────────────────────────
	// gross=102600 (1026.00 SEK)  net=102600 (1026.00 SEK)
	// ratio = 1/1, expected = 1/3 → ratio_excess = 3.0 → CRITICAL
	sb.WriteString(mwnLine("STIM20250003", "RECOVERY AFTER ADJ", "T2000000030") + "\n")
	sb.WriteString(mdrLine() + "\n")
	sb.WriteString(mipLine(10000, 10000) + "\n")
	sb.WriteString(werLine(102600, 102600) + "\n")

	// ── FEO: silently skipped ──────────────────────────────────────────────────
	sb.WriteString(rawRecord("FEO") + "\n")

	// ── Work 4: After FEO, CLEAN ──────────────────────────────────────────────
	sb.WriteString(mwnLine("STIM20250004", "RECOVERY AFTER FEO", "T2000000040") + "\n")
	sb.WriteString(mdrLine() + "\n")
	sb.WriteString(mipLine(10000, 10000) + "\n")
	sb.WriteString(werLine(300000, 100000) + "\n")

	// ── Work 5: ICC inside MWN block, CRITICAL ────────────────────────────────
	// ICC appears between MIP and WER — detection must ignore it and use WER only.
	sb.WriteString(mwnLine("STIM20250005", "ICC INSIDE BLOCK", "T2000000050") + "\n")
	sb.WriteString(mdrLine() + "\n")
	sb.WriteString(mipLine(10000, 10000) + "\n")
	sb.WriteString(rawRecord("ICC") + "\n") // must be silently ignored
	sb.WriteString(werLine(102600, 102600) + "\n")

	path := "testdata/verostark_edge_cases.crd"
	if err := os.WriteFile(path, []byte(sb.String()), 0644); err != nil {
		fmt.Fprintf(os.Stderr, "write %s: %v\n", path, err)
		os.Exit(1)
	}
	fmt.Printf("%-52s    5 works    5 WER lines\n", path)
	fmt.Printf("  CLEAN:2  CRITICAL:2  SKIPPED:3(ADJ+FEO+ICC)\n\n")
}

// ── File writer ───────────────────────────────────────────────────────────────

func writeFile(cfg fileConfig) {
	works := buildWorks(cfg)

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

	path := "testdata/" + cfg.filename
	if err := os.WriteFile(path, []byte(sb.String()), 0644); err != nil {
		fmt.Fprintf(os.Stderr, "write %s: %v\n", path, err)
		os.Exit(1)
	}

	werCount := 0
	for _, w := range works {
		werCount += len(w.events)
	}
	fmt.Printf("%-52s  %3d works  %3d WER lines\n", path, len(works), werCount)
}

// ── Main ──────────────────────────────────────────────────────────────────────

func main() {
	if err := os.MkdirAll("testdata", 0755); err != nil {
		fmt.Fprintf(os.Stderr, "mkdir: %v\n", err)
		os.Exit(1)
	}

	files := []fileConfig{
		{
			// Original stress file — equal spread across all severities.
			filename:  "verostark_stress_100_works.crd",
			seed:      42,
			refPrefix: "STIM100",
			clean:     25, possible: 15, medium: 20, high: 20, critical: 15, underpay: 5,
		},
		{
			// Sample A — well-managed catalogue, mostly clean.
			// Story: healthy statement with a handful of flags to investigate.
			filename:  "verostark_sample_clean_majority.crd",
			seed:      123,
			refPrefix: "STIM200",
			clean:     40, possible: 20, medium: 20, high: 15, critical: 5, underpay: 0,
		},
		{
			// Sample B — systematic deviation, many serious flags.
			// Story: territorial override pattern across most of the catalogue.
			filename:  "verostark_sample_systematic_deviation.crd",
			seed:      456,
			refPrefix: "STIM300",
			clean:     5, possible: 10, medium: 15, high: 35, critical: 35, underpay: 0,
		},
		{
			// Sample C — underpayment focus, publisher is being shortchanged.
			// Story: collecting society consistently paid below the expected share.
			filename:  "verostark_sample_underpayments.crd",
			seed:      789,
			refPrefix: "STIM400",
			clean:     25, possible: 10, medium: 10, high: 10, critical: 10, underpay: 35,
		},
	}

	fmt.Println("Generating CRD test files:")
	fmt.Println()
	for _, cfg := range files {
		writeFile(cfg)
		fmt.Printf("  CLEAN:%-3d  POSSIBLE:%-3d  MEDIUM:%-3d  HIGH:%-3d  CRITICAL:%-3d  UNDERPAY:%-3d\n\n",
			cfg.clean, cfg.possible, cfg.medium, cfg.high, cfg.critical, cfg.underpay)
	}
	writeEdgeCases()
}

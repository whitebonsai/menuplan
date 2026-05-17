package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"time"
)

const (
	baseURL  = "https://app.food2050.ch"
	basePath = "/de/v2/zfv/sbb/ostermundigen/mittagsverpflegung,hauptspeisen"

	weeksBack = 2
	weeksFwd  = 2
)

var (
	weekdaysDE    = [7]string{"Sonntag", "Montag", "Dienstag", "Mittwoch", "Donnerstag", "Freitag", "Samstag"}
	weekdaysShort = [7]string{"So", "Mo", "Di", "Mi", "Do", "Fr", "Sa"}
	monthsDE      = [13]string{"", "Januar", "Februar", "März", "April", "Mai", "Juni", "Juli", "August", "September", "Oktober", "November", "Dezember"}
	climateLabels = map[string]string{"LOW": "niedrig", "MEDIUM": "mittel", "HIGH": "hoch", "VERY_HIGH": "sehr hoch"}
	climateColors = map[string]string{"LOW": "#22c55e", "MEDIUM": "#eab308", "HIGH": "#f97316", "VERY_HIGH": "#ef4444"}
	buildIDRe     = regexp.MustCompile(`/_next/static/([^/]+)/_buildManifest\.js`)
)

// ── API response types ───────────────────────────────────────────────────────

type apiResponse struct {
	PageProps struct {
		Organisation struct {
			Outlet struct {
				MenuCategory struct {
					MenuItem struct {
						Dish apiDish `json:"dish"`
					} `json:"menuItem"`
				} `json:"menuCategory"`
			} `json:"outlet"`
		} `json:"organisation"`
	} `json:"pageProps"`
}

type apiDish struct {
	Name         string            `json:"name"`
	IsVegan      bool              `json:"isVegan"`
	IsVegetarian bool              `json:"isVegetarian"`
	Allergens    []allergenWrapper `json:"allergens"`
	Stats        stats             `json:"stats"`
}

type allergenWrapper struct {
	Allergen struct {
		Name string `json:"name"`
	} `json:"allergen"`
}

type stats struct {
	ClimateImpact struct {
		Rating            string  `json:"rating"`
		TemperatureChange float64 `json:"temperatureChange"`
	} `json:"food2050climateImpact"`
	HealthRating struct {
		IsBalanced bool `json:"isBalanced"`
	} `json:"food2050HealthRating"`
}

// ── Output types ─────────────────────────────────────────────────────────────

type weeksOutput struct {
	CurrentWeek string     `json:"currentWeek"`
	Weeks       []weekData `json:"weeks"`
}

type weekData struct {
	Monday string      `json:"monday"`
	Title  string      `json:"title"`
	Days   []menuEntry `json:"days"`
}

type menuEntry struct {
	Date          string     `json:"date"`
	Weekday       string     `json:"weekday"`
	WeekdayShort  string     `json:"weekdayShort"`
	DateFormatted string     `json:"dateFormatted"`
	Main          *dishEntry `json:"main"`
	Vegi          *dishEntry `json:"vegi"`
}

type dishEntry struct {
	Name         string   `json:"name"`
	IsVegan      bool     `json:"isVegan"`
	IsVegetarian bool     `json:"isVegetarian"`
	Allergens    []string `json:"allergens"`
	Climate      climate  `json:"climate"`
	IsBalanced   bool     `json:"isBalanced"`
}

type climate struct {
	Label      string `json:"label"`
	Color      string `json:"color"`
	TempChange string `json:"tempChange"`
}

// ── HTTP helper ───────────────────────────────────────────────────────────────

func get(url string) ([]byte, error) {
	req, _ := http.NewRequest("GET", url, nil)
	req.Header.Set("User-Agent", "Mozilla/5.0")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	return io.ReadAll(resp.Body)
}

// ── Build ID ──────────────────────────────────────────────────────────────────

func getBuildID() (string, error) {
	body, err := get(fmt.Sprintf("%s%s,menu/2026-01-01", baseURL, basePath))
	if err != nil {
		return "", err
	}
	m := buildIDRe.FindSubmatch(body)
	if m == nil {
		return "", fmt.Errorf("build ID not found in HTML")
	}
	return string(m[1]), nil
}

// ── Fetch one dish ────────────────────────────────────────────────────────────

func fetchDish(buildID, kind, date string) (*dishEntry, error) {
	url := fmt.Sprintf("%s/_next/data/%s%s,%s/%s.json", baseURL, buildID, basePath, kind, date)
	body, err := get(url)
	if err != nil {
		return nil, err
	}

	var resp apiResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, err
	}

	d := resp.PageProps.Organisation.Outlet.MenuCategory.MenuItem.Dish
	if d.Name == "" {
		return nil, fmt.Errorf("no dish found")
	}

	allergens := make([]string, len(d.Allergens))
	for i, a := range d.Allergens {
		allergens[i] = a.Allergen.Name
	}

	cr := d.Stats.ClimateImpact.Rating
	return &dishEntry{
		Name:         d.Name,
		IsVegan:      d.IsVegan,
		IsVegetarian: d.IsVegetarian,
		Allergens:    allergens,
		Climate: climate{
			Label:      climateLabels[cr],
			Color:      climateColors[cr],
			TempChange: fmt.Sprintf("%.1f", d.Stats.ClimateImpact.TemperatureChange),
		},
		IsBalanced: d.Stats.HealthRating.IsBalanced,
	}, nil
}

// ── Date helpers ──────────────────────────────────────────────────────────────

func mondayOf(t time.Time) time.Time {
	days := int(t.Weekday()) - int(time.Monday)
	if days < 0 {
		days += 7
	}
	return t.AddDate(0, 0, -days).Truncate(24 * time.Hour)
}

func weekDates(monday time.Time) []time.Time {
	dates := make([]time.Time, 5)
	for i := range dates {
		dates[i] = monday.AddDate(0, 0, i)
	}
	return dates
}

func fmtDate(d time.Time) string {
	return fmt.Sprintf("%d. %s", d.Day(), monthsDE[d.Month()])
}

func defaultWeek() time.Time {
	today := time.Now()
	monday := mondayOf(today)
	if today.Weekday() == time.Saturday || today.Weekday() == time.Sunday {
		monday = monday.AddDate(0, 0, 7)
	}
	return monday
}

// ── Cache ─────────────────────────────────────────────────────────────────────

func cacheFile(cacheDir, monday string) string {
	return filepath.Join(cacheDir, monday+".json")
}

// cacheValid returns true if the cached week can be used without re-fetching.
// Past weeks: valid forever. Future/current weeks: valid for 7 days.
func cacheValid(path string, monday time.Time) bool {
	info, err := os.Stat(path)
	if err != nil {
		return false
	}
	// Past week: the whole week has already happened.
	if monday.AddDate(0, 0, 7).Before(time.Now()) {
		return true
	}
	return time.Since(info.ModTime()) < 7*24*time.Hour
}

func loadCache(path string) (*weekData, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var wd weekData
	if err := json.Unmarshal(b, &wd); err != nil {
		return nil, err
	}
	return &wd, nil
}

func saveCache(path string, wd *weekData) error {
	b, err := json.MarshalIndent(wd, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, b, 0644)
}

// ── Fetch one week (with cache) ───────────────────────────────────────────────

func fetchWeek(buildID, cacheDir string, monday time.Time) weekData {
	cf := cacheFile(cacheDir, monday.Format("2006-01-02"))

	if cacheValid(cf, monday) {
		if wd, err := loadCache(cf); err == nil {
			fmt.Printf("    (cache hit)\n")
			return *wd
		}
	}

	dates := weekDates(monday)
	friday := dates[4]
	title := fmt.Sprintf("%s – %s %d", fmtDate(monday), fmtDate(friday), monday.Year())

	var days []menuEntry
	for _, d := range dates {
		date := d.Format("2006-01-02")
		weekday := weekdaysDE[d.Weekday()]
		fmt.Printf("    %s %s...\n", weekday, date)

		main, err := fetchDish(buildID, "menu", date)
		if err != nil {
			fmt.Printf("      main: %v\n", err)
		}
		vegi, err := fetchDish(buildID, "vegi", date)
		if err != nil {
			fmt.Printf("      vegi: %v\n", err)
		}

		if main != nil || vegi != nil {
			days = append(days, menuEntry{
				Date:          date,
				Weekday:       weekday,
				WeekdayShort:  weekdaysShort[d.Weekday()],
				DateFormatted: fmtDate(d),
				Main:          main,
				Vegi:          vegi,
			})
		}
	}

	wd := weekData{
		Monday: monday.Format("2006-01-02"),
		Title:  title,
		Days:   days,
	}

	if err := saveCache(cf, &wd); err != nil {
		fmt.Printf("    warning: cache write failed: %v\n", err)
	}
	return wd
}

// ── Main ──────────────────────────────────────────────────────────────────────

func main() {
	cacheDir := flag.String("cache-dir", "./cache", "directory for cached week JSON files")
	flag.Parse()

	if err := os.MkdirAll(*cacheDir, 0755); err != nil {
		log.Fatal(err)
	}

	fmt.Println("Fetching build ID...")
	buildID, err := getBuildID()
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("  Build ID: %s\n", buildID)

	defWeek := defaultWeek()
	var weeks []weekData

	for offset := -weeksBack; offset <= weeksFwd; offset++ {
		monday := defWeek.AddDate(0, 0, offset*7)
		fmt.Printf("\nWeek %+d: %s\n", offset, monday.Format("2006-01-02"))
		weeks = append(weeks, fetchWeek(buildID, *cacheDir, monday))
	}

	output := weeksOutput{
		CurrentWeek: defWeek.Format("2006-01-02"),
		Weeks:       weeks,
	}

	if err := os.MkdirAll("data", 0755); err != nil {
		log.Fatal(err)
	}
	out, err := json.MarshalIndent(output, "", "  ")
	if err != nil {
		log.Fatal(err)
	}
	if err := os.WriteFile("data/weeks.json", out, 0644); err != nil {
		log.Fatal(err)
	}

	total := 0
	for _, w := range weeks {
		total += len(w.Days)
	}
	fmt.Printf("\nSaved %d weeks (%d days) to data/weeks.json\n", len(weeks), total)
}

func truncate(s string, n int) string {
	if len(s) > n {
		return s[:n]
	}
	return s
}

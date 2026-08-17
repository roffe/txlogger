package common

import (
	"cmp"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"
)

var EcuList = []string{"T5", "T7", "T8", "AW55"}

const (
	Pi15            = math.Pi * 1.5
	Pi43            = math.Pi / 4 * 3
	PiDiv180        = math.Pi / 180
	OneOneFive      = 1.0 / 1.5    // 0.6666666666666666
	OneOneEight     = 1.0 / 1.8    // 0.5555555555555556
	OneHalf         = 1.0 / 2.0    // 0.5
	OneHalfOne      = 1.0 / 2.1    // 0.47619047619047616
	OneHalfSix      = 1.0 / 2.6    // 0.38461538461538464
	OneThird        = 1.0 / 3.0    // 0.3333333333333333
	OneThirdAndHalf = 1.0 / 3.5    // 0.2857142857142857
	OneFourth       = 1.0 / 4.0    // 0.25
	OneFifth        = 1.0 / 5.0    // 0.2
	OneSixth        = 1.0 / 6.0    // 0.16666666666666666
	OneSeventh      = 1.0 / 7.0    // 0.14285714285714285
	OneEight        = 1.0 / 8.0    // 0.125
	OneTenth        = 1.0 / 10.0   // 0.1
	OneTwelfth      = 1.0 / 12.0   // 0.08333333333333333
	OneTwentieth    = 1.0 / 20.0   // 0.05
	OneTwentyFifth  = 1.0 / 25.0   // 0.04
	OneSixthieth    = 1.0 / 60.0   // 0.016666666666666666
	OneEighthieth   = 1.0 / 80.0   // 0.0125
	OneTwohundredth = 1.0 / 200.0  // 0.005
	OneTwelvehundth = 1.0 / 1200.0 // 0.0008333333333333334
)

var safeNameRE = regexp.MustCompile(`[^\w.\-]+`) // allow letters, numbers, _, -, .

func SanitizeFilename(name string) string {
	name = filepath.Base(name) // strip directories
	name = safeNameRE.ReplaceAllString(name, "_")
	if name == "" || name == "." || name == ".." {
		name = fmt.Sprintf("file_%d", time.Now().Unix())
	}
	return name
}

func ListFilesInPathByExtension(path, ext string) ([]string, error) {
	entries, err := os.ReadDir(path)
	if err != nil {
		return nil, err
	}
	var files []string
	for _, entry := range entries {
		if !entry.IsDir() && strings.EqualFold(filepath.Ext(entry.Name()), ext) {
			files = append(files, entry.Name())
		}
	}
	return files, nil
}

func GetLogPath() (string, error) {
	dir, err := GetUserHomeDir()
	if err != nil {
		return "", err
	}
	logPath := GetComponentPath(dir, "logs")
	return logPath, createDirIfNotExists(logPath)
}

func GetADScannerPath() (string, error) {
	dir, err := GetUserHomeDir()
	if err != nil {
		return "", err
	}
	adScannerPath := GetComponentPath(dir, "adscanner")
	return adScannerPath, createDirIfNotExists(adScannerPath)
}

func GetLayoutPath() (string, error) {
	dir, err := GetUserHomeDir()
	if err != nil {
		return "", err
	}
	layoutPath := GetComponentPath(dir, "layouts")
	return layoutPath, createDirIfNotExists(layoutPath)
}

func GetDashboardPath() (string, error) {
	dir, err := GetUserHomeDir()
	if err != nil {
		return "", err
	}
	dashboardPath := GetComponentPath(dir, "dashboards")
	return dashboardPath, createDirIfNotExists(dashboardPath)
}

func GetMatrixBuilderPath() (string, error) {
	dir, err := GetUserHomeDir()
	if err != nil {
		return "", err
	}
	matrixBuilderPath := GetComponentPath(dir, "matrixbuilder")
	return matrixBuilderPath, createDirIfNotExists(matrixBuilderPath)
}

func GetScriptsPath() (string, error) {
	dir, err := GetUserHomeDir()
	if err != nil {
		return "", err
	}
	scriptsPath := GetComponentPath(dir, "scripts")
	return scriptsPath, createDirIfNotExists(scriptsPath)
}

func GetBinPath() (string, error) {
	dir, err := GetUserHomeDir()
	if err != nil {
		return "", err
	}
	binPath := GetComponentPath(dir, "bins")
	return binPath, createDirIfNotExists(binPath)
}

func GetComponentPath(base, typ string) string {
	return filepath.Join(base, "txlogger", typ)
}

func GetUserHomeDir() (string, error) {
	dir, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("could not determine user home directory: %v", err)
	}
	return dir, nil
}

func CreatetxloggerDirs() error {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("could not determine user home directory: %w", err)
	}
	path := filepath.Join(homeDir, "txlogger")

	logs := filepath.Join(path, "logs")
	layouts := filepath.Join(path, "layouts")
	bins := filepath.Join(path, "bins")
	adscanner := filepath.Join(path, "adscanner")
	dashboards := filepath.Join(path, "dashboards")

	dirs := []string{logs, layouts, bins, adscanner, dashboards}
	for _, ecu := range EcuList {
		dirs = append(dirs, filepath.Join(path, "scripts", ecu))
	}

	for _, p := range dirs {
		if err := createDirIfNotExists(p); err != nil {
			return err
		}
	}

	return nil
}

func createDirIfNotExists(path string) error {
	if err := os.MkdirAll(path, os.ModePerm); err != nil {
		return fmt.Errorf("could not create directory %s: %v", path, err)
	}
	return nil
}

func ParseFixedPrec(format string) int {
	if len(format) >= 4 && format[0] == '%' && format[1] == '.' && format[len(format)-1] == 'f' {
		n := 0
		has := false
		for i := 2; i < len(format)-1; i++ {
			ch := format[i]
			if ch < '0' || ch > '9' {
				return -1
			}
			has = true
			n = n*10 + int(ch-'0')
		}
		if has {
			return n
		}
	}
	return -1
}

func AppendFormatFloat(dst []byte, format string, v float64) []byte {
	if n := ParseFixedPrec(format); n >= 0 {
		return strconv.AppendFloat(dst, v, 'f', n, 64)
	}
	return fmt.Appendf(dst, format, v)
}

// SameTextBytes reports whether s and b have identical contents, without allocating.
func SameTextBytes(s string, b []byte) bool {
	if len(s) != len(b) {
		return false
	}
	for i, v := range b {
		if s[i] != v {
			return false
		}
	}
	return true
}

func Abs(n int) int {
	if n < 0 {
		return -n
	}
	return n
}

type Number interface {
	~int | ~int8 | ~int16 | ~int32 | ~int64 | ~uint | ~uint8 | ~uint16 | ~uint32 | ~uint64 | ~float32 | ~float64
}

// Generic function that works with any numeric type
func FindMinMax[T Number](data []T) (T, T) {
	if len(data) == 0 {
		panic("empty slice")
	}

	min, max := data[0], data[0]
	for _, v := range data {
		if v < min {
			min = v
		}
		if v > max {
			max = v
		}
	}
	return min, max
}

func Clamp[T cmp.Ordered](value, min, max T) T {
	if value < min {
		return min
	}
	if value > max {
		return max
	}
	return value
}

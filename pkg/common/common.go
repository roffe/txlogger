package common

import (
	"fmt"
	"math"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"time"
)

var EcuList = []string{"T5", "T7", "T8"}

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

func GetLogPath() (string, error) {
	dir, err := getUserHomeDir()
	if err != nil {
		return "", err
	}
	logPath := getComponentPath(dir, "logs")
	return logPath, createDirIfNotExists(logPath)
}

func GetBinPath() (string, error) {
	dir, err := getUserHomeDir()
	if err != nil {
		return "", err
	}
	binPath := getComponentPath(dir, "bins")
	return binPath, createDirIfNotExists(binPath)
}

func getComponentPath(base, typ string) string {
	return filepath.Join(base, "txlogger", typ)
}

func getUserHomeDir() (string, error) {
	dir, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("could not determine user home directory: %v", err)
	}
	return dir, nil
}

func createDirIfNotExists(path string) error {
	if err := os.MkdirAll(path, os.ModePerm); err != nil {
		return fmt.Errorf("could not create log directory: %v", err)
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
	return strconv.AppendFloat(dst, v, 'f', 0, 64)
}

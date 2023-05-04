package main

import (
	"bufio"
	"fmt"
	"go/token"
	"go/types"
	"log"
	"os"
	"strings"
)

func main() {

	f, err := os.Create("fixad32.t7l")
	if err != nil {
		log.Fatal(err)
	}
	defer f.Close()

	readFile, err := os.Open("log-2023-05-03-19-43-55.t7l")
	if err != nil {
		log.Fatal(err)
	}
	defer readFile.Close()
	fileScanner := bufio.NewScanner(readFile)

	fileScanner.Split(bufio.ScanLines)

	for fileScanner.Scan() {
		var newLine strings.Builder
		line := fileScanner.Text()
		parts := strings.Split(line, "|")
		//fmt.Println(parts)
		newLine.WriteString(parts[0] + "|")

		for _, part := range parts[1:] {

			kv := strings.Split(part, "=")
			if len(kv) != 2 {
				continue
			}
			switch kv[0] {
			case "IgnProt.fi_Offset", "Out.X_AccPedal", "Out.fi_Ignition", "Out.PWM_BoostCntrl", "In.v_Vehicle", "In.p_AirAmbient", "In.p_AirInlet":
				newLine.WriteString(korv(kv[0], kv[1], "0.1") + "|")
			case "ECMStat.p_Diff", "ECMStat.p_DiffThrot", "In.p_AirBefThrottle":
				newLine.WriteString(korv(kv[0], kv[1], "0.001") + "|")
			default:
				newLine.WriteString(kv[0] + "=" + kv[1] + "|")
			}
		}
		//fmt.Println(newLine.String())
		f.WriteString(newLine.String() + "\n")
	}
}

func korv(name, value string, factor string) string {
	fs := token.NewFileSet()
	tv, err := types.Eval(fs, nil, token.NoPos, fmt.Sprintf("%s*%s", value, factor))
	if err != nil {
		panic(err)
	}
	return fmt.Sprintf("%s=%s", name, strings.ReplaceAll(tv.Value.String(), ".", ","))
}

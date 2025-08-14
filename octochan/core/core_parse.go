package core

import (
	"fmt"
	"math"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
)

func Normalize_value(value interface{}) interface{} {
	if value == nil {
		return nil
	}

	orig_val := fmt.Sprintf("%v", value)
	re := regexp.MustCompile(`\s*#.*$`)
	clean := re.ReplaceAllString(orig_val, "")
	clean = strings.TrimSpace(strings.ToLower(clean))
	if clean == "" {
		return nil
	}

	special_char := []string{"/", "@", "$", "*", "^", "(", ")"}
	if Any(special_char, func(c string) bool {
		return strings.Contains(clean, c)
	}) {
		return clean
	}

	bool_maping := map[string]string{
		"yes":     "on",
		"no":      "off",
		"true":    "on",
		"false":   "off",
		"1":       "on",
		"0":       "off",
		"enable":  "on",
		"disable": "off",
	}
	if val, ok := bool_maping[clean]; ok {
		return val
	}

	sizePattern := regexp.MustCompile(`^(\d+\.?\d*)\s*([a-z]*)$`)
	sizeMatches := sizePattern.FindStringSubmatch(clean)
	if len(sizeMatches) > 0 {
		numStr := sizeMatches[1]
		unit := sizeMatches[2]

		num, err := strconv.ParseFloat(numStr, 64)
		if err != nil {
			return clean
		}

		if unit == "" {
			unit = "b"
		}

		sizeUnits := map[string]float64{
			"b":  1,
			"kb": 1024, "k": 1024,
			"mb": math.Pow(1024, 2), "m": math.Pow(1024, 2),
			"gb": math.Pow(1024, 3), "g": math.Pow(1024, 3),
			"tb": math.Pow(1024, 4), "t": math.Pow(1024, 4),
			"pb": math.Pow(1024, 5), "p": math.Pow(1024, 5),
		}

		if multiplier, ok := sizeUnits[unit]; ok {
			bytesVal := num * multiplier

			sortedUnits := []struct {
				name  string
				value float64
			}{
				{"pb", sizeUnits["pb"]},
				{"tb", sizeUnits["tb"]},
				{"gb", sizeUnits["gb"]},
				{"mb", sizeUnits["mb"]},
				{"kb", sizeUnits["kb"]},
				{"b", sizeUnits["b"]},
			}

			for _, u := range sortedUnits {
				if bytesVal >= u.value || u.name == "b" {
					normalized := bytesVal / u.value
					switch u.name {
					case "b":
						return fmt.Sprintf("%dB", int(normalized))
					case "k", "kb":
						return fmt.Sprintf("%.0fKB", normalized)
					case "m", "mb":
						if bytesVal < 10*math.Pow(1024, 2) {
							return fmt.Sprintf("%.1fMB", normalized)
						} else {
							return fmt.Sprintf("%.0fMB", normalized)
						}
					case "g", "gb":
						if bytesVal < 10*math.Pow(1024, 3) {
							return fmt.Sprintf("%.1fGB", normalized)
						} else {
							return fmt.Sprintf("%.0fGB", normalized)
						}
					case "t", "tb":
						return fmt.Sprintf("%.2fTB", normalized)
					default:
						return fmt.Sprintf("%.3fPB", normalized)
					}
				}
			}
		}
	}
	if f, err := strconv.ParseFloat(clean, 64); err == nil {
		if f == math.Trunc(f) {
			return fmt.Sprintf("%d", int(f))
		}
		return fmt.Sprintf("%v", f)
	}

	return clean
}

func ParseConfig(fileContent interface{}) (map[string]map[string]string, error) {
	config := make(map[string]map[string]string)
	var currentSection string

	content, ok := fileContent.(string)
	if !ok {
		return nil, fmt.Errorf("file_content must be a string, got %T", fileContent)
	}

	lines := strings.Split(content, "\n")
	lineRe := regexp.MustCompile(`\s*#.*$`)

	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		line = lineRe.ReplaceAllString(line, "")
		line = strings.TrimSpace(line)

		if strings.HasPrefix(line, "[") && strings.HasSuffix(line, "]") {
			currentSection = line[1 : len(line)-1]
			if _, exists := config[currentSection]; !exists {
				config[currentSection] = make(map[string]string)
			}
			continue
		}

		if eqIndex := strings.Index(line, "="); eqIndex > 0 {
			key := strings.TrimSpace(line[:eqIndex])
			value := strings.TrimSpace(line[eqIndex+1:])
			value = strings.Trim(value, "'\"")

			if currentSection != "" {
				if _, exists := config[currentSection]; !exists {
					config[currentSection] = make(map[string]string)
				}
				config[currentSection][key] = value
			} else {
				if _, exists := config[""]; !exists {
					config[""] = make(map[string]string)
				}
				config[""][key] = value
			}
		}
	}

	return config, nil
}

func CompareConfigs(db1Config, db2Config map[string]map[string]string, db1Name, db2Name string) map[string]map[string]interface{} {
	diff := make(map[string]map[string]interface{})
	allKeys := make(map[string]bool)

	for key := range db1Config {
		allKeys[key] = true
	}
	for key := range db2Config {
		allKeys[key] = true
	}

	var sortedKeys []string
	for key := range allKeys {
		sortedKeys = append(sortedKeys, key)
	}
	sort.Strings(sortedKeys)

	for _, key := range sortedKeys {
		db1Val, db1Exists := db1Config[key]
		db2Val, db2Exists := db2Config[key]

		var normDb1, normDb2 interface{}

		if db1Exists {
			normDb1 = Normalize_value(db1Val)
		} else {
			normDb1 = nil
		}

		if db2Exists {
			normDb2 = Normalize_value(db2Val)
		} else {
			normDb2 = nil
		}

		if !equalValues(normDb1, normDb2) {
			status := ""
			if db1Exists && db2Exists {
				status = "Modified"
			} else if db1Exists {
				status = fmt.Sprintf("Only in %s", db1Name)
			} else {
				status = fmt.Sprintf("Only in %s", db2Name)
			}

			diffEntry := make(map[string]interface{})
			diffEntry[db1Name] = getValueOrNA(db1Val)
			diffEntry[db2Name] = getValueOrNA(db2Val)
			diffEntry["status"] = status
			diffEntry["norm1"] = normDb1
			diffEntry["norm2"] = normDb2

			diff[key] = diffEntry
		}
	}

	return diff
}

func SaveDiffToFile(diff map[string]map[string]interface{}, outputFile, db1Name, db2Name string) error {
	f, err := os.Create(outputFile)
	if err != nil {
		return fmt.Errorf("failed to create output file: %v", err)
	}
	defer f.Close()

	currentTime := time.Now().Format("2006-01-02 15:04:05")
	header := fmt.Sprintf("Comparison report: %s vs %s\n", db1Name, db2Name)
	header += fmt.Sprintf("Generated at: %s\n\n", currentTime)
	header += fmt.Sprintf("%-50s | %-30s | %-30s | %-15s\n", "Parameter", db1Name, db2Name, "Status")
	header += fmt.Sprintf("%s\n", strings.Repeat("-", 125))

	if _, err := f.WriteString(header); err != nil {
		return fmt.Errorf("failed to write header: %v", err)
	}

	type diffItem struct {
		param string
		data  map[string]interface{}
	}
	var items []diffItem

	for param, data := range diff {
		items = append(items, diffItem{param, data})
	}

	sort.Slice(items, func(i, j int) bool {
		statusI := items[i].data["status"].(string)
		statusJ := items[j].data["status"].(string)

		if statusI == "Modified" && statusJ != "Modified" {
			return true
		}
		if statusI != "Modified" && statusJ == "Modified" {
			return false
		}
		if statusI == fmt.Sprintf("Only in %s", db1Name) && statusJ == fmt.Sprintf("Only in %s", db2Name) {
			return true
		}
		return false
	})

	for _, item := range items {
		param := item.param
		data := item.data

		valDB1 := "N/A"
		if v, ok := data[db1Name]; ok && v != "N/A" {
			valDB1 = fmt.Sprintf("%v", v)
		}

		valDB2 := "N/A"
		if v, ok := data[db2Name]; ok && v != "N/A" {
			valDB2 = fmt.Sprintf("%v", v)
		}

		status := data["status"].(string)
		line := fmt.Sprintf("%-50s | %-30s | %-30s | %-15s\n", param, valDB1, valDB2, status)

		if _, err := f.WriteString(line); err != nil {
			return fmt.Errorf("failed to write diff line: %v", err)
		}
	}

	return nil
}
func GetNextReportNumber(reportDir string) (int, error) {
	files, err := os.ReadDir(reportDir)
	if err != nil {
		return 0, fmt.Errorf("failed to read report directory: %v", err)
	}

	var numbers []int
	prefix := "diff_report_"
	suffix := ".txt"

	for _, file := range files {
		if file.IsDir() {
			continue
		}

		name := file.Name()
		if !strings.HasPrefix(name, prefix) || !strings.HasSuffix(name, suffix) {
			continue
		}

		numStr := name[len(prefix) : len(name)-len(suffix)]
		num, err := strconv.Atoi(numStr)
		if err != nil {
			continue
		}

		numbers = append(numbers, num)
	}

	if len(numbers) == 0 {
		return 1, nil
	}

	sort.Ints(numbers)
	return numbers[len(numbers)-1] + 1, nil
}

func ParsePgHba(line string) map[string]string {

	trimmed := strings.TrimSpace(line)
	if trimmed == "" {
		return nil
	}

	re := regexp.MustCompile(`\s+`)
	parts := re.Split(trimmed, -1)

	if len(parts) < 4 {
		return nil
	}

	result := make(map[string]string)
	result["type"] = parts[0]
	result["database"] = parts[1]
	result["user"] = parts[2]
	result["address"] = parts[3]

	if len(parts) > 4 {
		result["method"] = parts[4]
	} else {
		result["method"] = ""
	}

	return result
}

func GetScriptDir() (string, error) {
	exe, err := os.Executable()
	if err != nil {
		return "", fmt.Errorf("не удалось определить директорию скрипта: %v", err)
	}
	return filepath.Dir(exe), nil
}

func ShowHeader(scriptDir string) {
	fmt.Println("Конфигурационный файл сравнения БД")
	fmt.Println("----------------------------------")
	fmt.Printf("Текущая директория скрипта: %s\n", scriptDir)
	fmt.Println("Доступные файлы в директории:")

	files, err := os.ReadDir(scriptDir)
	if err != nil {
		fmt.Printf("Ошибка при чтении директории: %v\n", err)
		return
	}

	for _, f := range files {
		name := f.Name()
		ext := strings.ToLower(filepath.Ext(name))
		if ext == ".conf" || ext == ".txt" || ext == ".cfg" {
			fmt.Printf("  - %s\n", name)
		}
	}
	fmt.Println()
}

func GetInputFiles(scriptDir string) (string, string) {
	var db1File, db2File string

	fmt.Print("Введите имя файла БД1 (например, prod.conf): ")
	fmt.Scanln(&db1File)
	fmt.Print("Введите имя файла БД2 (например, test.conf): ")
	fmt.Scanln(&db2File)

	db1File = ResolveFilePath(scriptDir, db1File)
	db2File = ResolveFilePath(scriptDir, db2File)

	if !FileExists(db1File) {
		exitWithError(fmt.Errorf("файл %s не найден", db1File))
	}
	if !FileExists(db2File) {
		exitWithError(fmt.Errorf("файл %s не найден", db2File))
	}

	return db1File, db2File
}

func ShowResults(diff map[string]map[string]interface{}, outputFile string) {
	if outputFile != "" {
		fmt.Printf("\nНайдены различия. Результаты сохранены в: %s\n", outputFile)
		fmt.Printf("Всего различий: %d\n", len(diff))
	} else {
		fmt.Println("\nКонфигурационные файлы идентичны, различий не найдено.")
	}
}

func ResolveFilePath(baseDir, filePath string) string {
	if filepath.IsAbs(filePath) {
		return filePath
	}
	return filepath.Join(baseDir, filePath)
}

func FileExists(path string) bool {
	_, err := os.Stat(path)
	return !os.IsNotExist(err)
}

func ReadFile(filePath string) (string, error) {
	content, err := os.ReadFile(filePath)
	if err != nil {
		return "", err
	}
	return string(content), nil
}

func SaveConfig(config map[string]map[string]string, filePath string) error {
	content := ""
	for section, params := range config {
		if section != "" {
			content += fmt.Sprintf("[%s]\n", section)
		}
		for key, value := range params {
			content += fmt.Sprintf("%s = %s\n", key, value)
		}
		content += "\n"
	}

	return os.WriteFile(filePath, []byte(content), 0644)
}

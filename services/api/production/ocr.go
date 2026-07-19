package production

import (
	"bytes"
	"context"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"unicode"
)

type OCRWord struct {
	Text       string  `json:"text"`
	Confidence float64 `json:"confidence"`
	X          int     `json:"x"`
	Y          int     `json:"y"`
	Width      int     `json:"width"`
	Height     int     `json:"height"`
}

type FontCandidate struct {
	Family     string  `json:"family"`
	Confidence float64 `json:"confidence"`
	Reason     string  `json:"reason"`
}

// OCRReport is advisory. Recognized characters never replace traced customer
// geometry automatically; high-confidence text is offered for editable rebuild.
type OCRReport struct {
	Available                  bool            `json:"available"`
	Attempted                  bool            `json:"attempted"`
	Text                       string          `json:"text,omitempty"`
	Confidence                 float64         `json:"confidence"`
	Language                   string          `json:"language,omitempty"`
	Words                      []OCRWord       `json:"words,omitempty"`
	FontCandidates             []FontCandidate `json:"fontCandidates,omitempty"`
	EditableRebuildRecommended bool            `json:"editableRebuildRecommended"`
	RequiresHumanConfirmation  bool            `json:"requiresHumanConfirmation"`
	Diagnostic                 string          `json:"diagnostic,omitempty"`
}

func (n NativeTools) RecognizeRasterText(ctx context.Context, img image.Image) OCRReport {
	tesseract := resolve(n.Tesseract, "tesseract")
	if tesseract == "" {
		return OCRReport{Available: false, Diagnostic: "Tesseract OCR is not installed"}
	}
	report := OCRReport{Available: true, Attempted: true, Language: "eng", RequiresHumanConfirmation: true}
	dir, err := os.MkdirTemp("", "printstudio-ocr-*")
	if err != nil {
		report.Diagnostic = err.Error()
		return report
	}
	defer os.RemoveAll(dir)
	input := filepath.Join(dir, "artwork.png")
	file, err := os.Create(input)
	if err != nil {
		report.Diagnostic = err.Error()
		return report
	}
	if err := png.Encode(file, img); err != nil {
		_ = file.Close()
		report.Diagnostic = err.Error()
		return report
	}
	_ = file.Close()
	cmd := exec.CommandContext(ctx, tesseract, input, "stdout", "-l", "eng", "--psm", "6", "tsv")
	data, err := cmd.Output()
	if err != nil {
		report.Diagnostic = fmt.Sprintf("OCR failed: %v", err)
		return report
	}
	report.Words, report.Text, report.Confidence = parseTesseractTSV(data)
	if strings.TrimSpace(report.Text) == "" {
		report.Diagnostic = "OCR found no readable text"
		return report
	}
	report.FontCandidates = inferFontCandidates(report.Text, report.Words)
	report.EditableRebuildRecommended = report.Confidence >= 88
	if report.Confidence < 70 {
		report.Diagnostic = "low-confidence OCR; keep traced geometry unless an operator confirms the characters"
	}
	return report
}

func binaryOCRImage(mask []bool, width, height int) *image.Gray {
	out := image.NewGray(image.Rect(0, 0, width, height))
	for i := range out.Pix {
		out.Pix[i] = 255
	}
	for i, foreground := range mask {
		if foreground {
			out.SetGray(i%width, i/width, color.Gray{Y: 0})
		}
	}
	return out
}

func parseTesseractTSV(data []byte) ([]OCRWord, string, float64) {
	lines := bytes.Split(data, []byte{'\n'})
	words := make([]OCRWord, 0)
	var text strings.Builder
	lastLine := ""
	weightedConfidence, weight := 0.0, 0
	for _, raw := range lines[1:] {
		fields := strings.Split(strings.TrimSpace(string(raw)), "\t")
		if len(fields) < 12 {
			continue
		}
		content := strings.TrimSpace(strings.Join(fields[11:], " "))
		confidence, err := strconv.ParseFloat(fields[10], 64)
		if err != nil || confidence < 0 || content == "" {
			continue
		}
		word := OCRWord{Text: content, Confidence: roundTo(confidence, 2)}
		word.X, _ = strconv.Atoi(fields[6])
		word.Y, _ = strconv.Atoi(fields[7])
		word.Width, _ = strconv.Atoi(fields[8])
		word.Height, _ = strconv.Atoi(fields[9])
		lineKey := fields[1] + "/" + fields[2] + "/" + fields[3] + "/" + fields[4]
		if text.Len() > 0 {
			if lineKey != lastLine {
				text.WriteByte('\n')
			} else {
				text.WriteByte(' ')
			}
		}
		text.WriteString(content)
		lastLine = lineKey
		words = append(words, word)
		characters := maxInt(1, len([]rune(content)))
		weightedConfidence += confidence * float64(characters)
		weight += characters
	}
	confidence := 0.0
	if weight > 0 {
		confidence = roundTo(weightedConfidence/float64(weight), 2)
	}
	return words, text.String(), confidence
}

func inferFontCandidates(text string, words []OCRWord) []FontCandidate {
	letters, uppercase := 0, 0
	for _, r := range text {
		if unicode.IsLetter(r) {
			letters++
			if unicode.IsUpper(r) {
				uppercase++
			}
		}
	}
	uppercaseRatio := 0.0
	if letters > 0 {
		uppercaseRatio = float64(uppercase) / float64(letters)
	}
	averageCharAspect, measurements := 0.0, 0
	for _, word := range words {
		characters := len([]rune(word.Text))
		if characters > 0 && word.Height > 0 {
			averageCharAspect += float64(word.Width) / float64(characters*word.Height)
			measurements++
		}
	}
	if measurements > 0 {
		averageCharAspect /= float64(measurements)
	}
	candidates := []FontCandidate{
		{Family: "Arial", Confidence: 0.48, Reason: "neutral sans-serif reconstruction baseline"},
		{Family: "Verdana", Confidence: 0.4, Reason: "wide sans-serif candidate"},
		{Family: "Trebuchet MS", Confidence: 0.36, Reason: "humanist sans-serif candidate"},
		{Family: "Georgia", Confidence: 0.31, Reason: "serif reconstruction candidate"},
		{Family: "Times New Roman", Confidence: 0.29, Reason: "compact serif candidate"},
	}
	if uppercaseRatio > 0.85 {
		candidates = append(candidates, FontCandidate{Family: "Impact", Confidence: 0.55, Reason: "predominantly uppercase display lettering"})
	}
	if averageCharAspect > 0.52 && averageCharAspect < 0.72 {
		candidates = append(candidates, FontCandidate{Family: "Courier New", Confidence: 0.42, Reason: "proportions are compatible with monospaced lettering"})
	}
	sort.SliceStable(candidates, func(i, j int) bool { return candidates[i].Confidence > candidates[j].Confidence })
	if len(candidates) > 5 {
		candidates = candidates[:5]
	}
	return candidates
}

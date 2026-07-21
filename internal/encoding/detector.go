package encoding

import (
	"encoding/base64"
	"fmt"
	"net/url"
	"strings"
	"unicode"
)

type Result struct {
	Param    string
	Value    string
	Decoded  string
	Encoding string
}

type Detector struct{}

func NewDetector() *Detector {
	return &Detector{}
}

func (d *Detector) Detect(param, raw string) []Result {
	var results []Result

	seen := map[string]bool{}

	add := func(encoding string, decoded string, ok bool) {
		if !ok {
			return
		}
		if decoded == raw {
			return
		}
		if !meaningful(decoded) {
			return
		}
		if seen[decoded] {
			return
		}
		seen[decoded] = true
		results = append(results, Result{
			Param:    param,
			Value:    raw,
			Decoded:  decoded,
			Encoding: encoding,
		})
	}

	if dec, ok := d.decodeURL(raw); ok {
		add("url", dec, true)
	}
	if dec, ok := d.decodeDoubleURL(raw); ok {
		add("double_url", dec, true)
	}
	if dec, ok := d.decodeBase64(raw); ok {
		add("base64", dec, true)
	}
	if dec, ok := d.decodeBase64URL(raw); ok {
		add("base64url", dec, true)
	}
	if dec, ok := d.decodeHTMLEntity(raw); ok {
		add("html_entity", dec, true)
	}
	if dec, ok := d.decodeUnicode(raw); ok {
		add("unicode", dec, true)
	}

	return results
}

func (d *Detector) decodeURL(raw string) (string, bool) {
	if !strings.Contains(raw, "%") {
		return "", false
	}

	decoded, err := url.QueryUnescape(raw)
	if err != nil {
		return "", false
	}
	return decoded, true
}

func (d *Detector) decodeDoubleURL(raw string) (string, bool) {
	if !strings.Contains(raw, "%") {
		return "", false
	}

	first, err := url.QueryUnescape(raw)
	if err != nil {
		return "", false
	}

	if first == raw {
		return "", false
	}

	if !strings.Contains(first, "%") {
		return "", false
	}

	decoded, err := url.QueryUnescape(first)
	if err != nil {
		return "", false
	}

	if decoded == first || decoded == raw {
		return "", false
	}

	return decoded, true
}

func (d *Detector) decodeBase64(raw string) (string, bool) {
	return d.tryBase64(raw, false)
}

func (d *Detector) decodeBase64URL(raw string) (string, bool) {
	return d.tryBase64(raw, true)
}

func (d *Detector) tryBase64(raw string, urlSafe bool) (string, bool) {
	cleaned := strings.TrimSpace(raw)

	if len(cleaned) < 4 {
		return "", false
	}

	padding := 4 - len(cleaned)%4
	if padding != 4 {
		cleaned += strings.Repeat("=", padding)
	}

	var decoded []byte
	var err error

	if urlSafe {
		decoded, err = base64.URLEncoding.DecodeString(cleaned)
	} else {
		decoded, err = base64.StdEncoding.DecodeString(cleaned)
	}

	if err != nil {
		if urlSafe {
			decoded, err = base64.RawURLEncoding.DecodeString(strings.TrimRight(cleaned, "="))
		} else {
			decoded, err = base64.RawStdEncoding.DecodeString(strings.TrimRight(cleaned, "="))
		}
	}
	if err != nil {
		return "", false
	}

	if len(decoded) < 2 {
		return "", false
	}

	result := string(decoded)

	if looksLikeRandom(result) {
		return "", false
	}

	return result, true
}

func (d *Detector) decodeHTMLEntity(raw string) (string, bool) {
	if !strings.Contains(raw, "&") {
		return "", false
	}

	decoded := decodeHTMLEntities(raw)
	if decoded == raw {
		return "", false
	}
	return decoded, true
}

func (d *Detector) decodeUnicode(raw string) (string, bool) {
	decoded := decodeUnicodeEscapes(raw)
	if decoded == raw {
		return "", false
	}
	if !meaningful(decoded) {
		return "", false
	}
	return decoded, true
}

func meaningful(s string) bool {
	if len(s) == 0 {
		return false
	}

	printable := 0
	nonPrintable := 0
	alpha := 0

	for _, r := range s {
		if r == ' ' || r == '\t' || r == '\n' {
			printable++
			continue
		}
		if unicode.IsPrint(r) {
			printable++
			if unicode.IsLetter(r) || unicode.IsDigit(r) {
				alpha++
			}
		} else {
			nonPrintable++
		}
	}

	if nonPrintable > printable {
		return false
	}

	if alpha == 0 {
		return false
	}

	return true
}

func looksLikeRandom(s string) bool {
	if len(s) < 2 {
		return false
	}

	nonAlpha := 0
	upper := 0
	lower := 0
	digit := 0
	special := 0

	for _, r := range s {
		switch {
		case unicode.IsUpper(r):
			upper++
		case unicode.IsLower(r):
			lower++
		case unicode.IsDigit(r):
			digit++
		case unicode.IsPunct(r) || unicode.IsSymbol(r):
			special++
		default:
			nonAlpha++
		}
	}

	total := len(s)

	if upper > 0 && lower > 0 && digit > 0 && special > 0 {
		if float64(upper+lower)/float64(total) < 0.3 {
			return true
		}
	}

	return false
}

var htmlEntities = map[string]string{
	"&amp;":  "&",
	"&lt;":   "<",
	"&gt;":   ">",
	"&quot;": "\"",
	"&#39;":  "'",
	"&#x27;": "'",
	"&#x2F;": "/",
	"&#47;":  "/",
	"&nbsp;": " ",
	"&apos;": "'",
}

func decodeHTMLEntities(s string) string {
	for entity, char := range htmlEntities {
		s = strings.ReplaceAll(s, entity, char)
	}

	s = decodeNumericEntities(s)

	return s
}

func decodeNumericEntities(s string) string {
	var result strings.Builder
	i := 0
	for i < len(s) {
		if s[i] == '&' && i+3 < len(s) && s[i+1] == '#' {
			j := i + 2
			isHex := false
			if s[j] == 'x' || s[j] == 'X' {
				isHex = true
				j++
			}
			start := j
			for j < len(s) && s[j] != ';' {
				j++
			}
			if j < len(s) && j > start {
				numStr := s[start:j]
				if isHex {
					var code64 int64
					if _, err := fmt.Sscanf(numStr, "%x", &code64); err == nil && code64 > 0 {
						result.WriteRune(rune(code64))
						i = j + 1
						continue
					}
				} else {
					var code rune
					if _, err := fmt.Sscanf(numStr, "%d", &code); err == nil && code > 0 {
						result.WriteRune(code)
						i = j + 1
						continue
					}
				}
			}
		}
		result.WriteByte(s[i])
		i++
	}
	return result.String()
}

func decodeUnicodeEscapes(s string) string {
	var result strings.Builder
	i := 0
	for i < len(s) {
		if i+5 < len(s) && s[i] == '\\' && (s[i+1] == 'u' || s[i+1] == 'U') {
			hexStr := s[i+2 : i+6]
			var code rune
			_, err := fmt.Sscanf(hexStr, "%04x", &code)
			if err == nil && code > 0 {
				result.WriteRune(code)
				i += 6
				continue
			}
		}

		if i+5 < len(s) && s[i] == '%' && s[i+1] == 'u' {
			hexStr := s[i+2 : i+6]
			var code rune
			_, err := fmt.Sscanf(hexStr, "%04x", &code)
			if err == nil && code > 0 {
				result.WriteRune(code)
				i += 6
				continue
			}
		}

		result.WriteByte(s[i])
		i++
	}
	return result.String()
}

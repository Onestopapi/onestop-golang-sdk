package onestop

import (
	"crypto/sha256"
	"errors"
	"math"
	"math/rand"
	"strconv"
	"strings"
)

// AkamaiSecCptCall holds the inputs for AkamaiSecCptPayload. Cookie is the
// sec_cpt cookie value; the rest come from the challenge JSON.
type AkamaiSecCptCall struct {
	Cookie, Token, Nonce string
	Timestamp            int64
	Difficulty, Count    int
}

// AkamaiSecCptPayload solves the sec-cpt crypto challenge locally and returns
// the body to POST to /_sec/verify?provider=<provider> (the challenge's provider,
// crypto or adaptive). No client or API key is needed.
func AkamaiSecCptPayload(call AkamaiSecCptCall) (string, error) {
	prefix, _, found := strings.Cut(call.Cookie, "~")
	if !found {
		return "", errors.New(`akamai: sec_cpt cookie must contain "~"`)
	}
	if call.Token == "" || call.Nonce == "" || call.Timestamp <= 0 {
		return "", errors.New("akamai: token, nonce and a positive timestamp are required")
	}
	if call.Difficulty <= 0 || call.Count <= 0 {
		return "", errors.New("akamai: difficulty and count must be positive")
	}
	seed := prefix + strconv.FormatInt(call.Timestamp, 10) + call.Nonce
	answers := make([]string, call.Count)
	for i := range answers {
		target := call.Difficulty + i
		base := seed + strconv.Itoa(target)
		for {
			answer := secCptCandidate(float64(rand.Uint64()>>11) / (1 << 53))
			if secCptRemainder(sha256.Sum256([]byte(base+answer)), target) == 0 {
				answers[i] = answer
				break
			}
		}
	}
	return `{"token": "` + call.Token + `", "answers": ["` + strings.Join(answers, `","`) + `"]}`, nil
}

// secCptCandidate formats value in [0, 1) the way the browser's
// Number.prototype.toString(16) does.
func secCptCandidate(value float64) string {
	delta := math.Max(math.SmallestNonzeroFloat64, 0.5*(math.Nextafter(value, 1)-value))
	fraction := value
	var digits []int
	for fraction >= delta {
		fraction *= 16
		delta *= 16
		digit := int(fraction)
		fraction -= float64(digit)
		digits = append(digits, digit)
		if (fraction > 0.5 || (fraction == 0.5 && digit&1 == 1)) && fraction+delta > 1 {
			for len(digits) > 0 && digits[len(digits)-1] == 15 {
				digits = digits[:len(digits)-1]
			}
			if len(digits) == 0 {
				return "1"
			}
			digits[len(digits)-1]++
			break
		}
	}
	if len(digits) == 0 {
		return "0"
	}
	text := make([]byte, len(digits))
	for i, d := range digits {
		text[i] = "0123456789abcdef"[d]
	}
	return "0." + string(text)
}

func secCptRemainder(sum [sha256.Size]byte, target int) int {
	remainder := 0
	for _, b := range sum {
		remainder = ((remainder << 8) | int(b)) % target
	}
	return remainder
}

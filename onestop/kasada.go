package onestop

import (
	cryptoRand "crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"math/rand"
	"strconv"
	"time"
)

// KasadaCDCall holds the inputs for KasadaCDHeader. CryptoCustomerKey,
// SeedSuffix, Difficulty and SubchallengeCount come from KasadaPScript.
type KasadaCDCall struct {
	ST, RST                           int64
	Mobile                            bool
	CT, CryptoCustomerKey, SeedSuffix string
	Difficulty, SubchallengeCount     int
}

// cdMobile and cdDesktop are the x-kpsdk-cd header JSON; field order is the wire order.
type cdMobile struct {
	WorkTime int64   `json:"workTime"`
	RST      int64   `json:"rst"`
	Duration float64 `json:"duration"`
	ID       string  `json:"id"`
	ST       int64   `json:"st"`
	Answers  []int64 `json:"answers"`
	D        int64   `json:"d"`
}

type cdDesktop struct {
	WorkTime int64   `json:"workTime"`
	ID       string  `json:"id"`
	Answers  []int64 `json:"answers"`
	Duration float64 `json:"duration"`
	D        int64   `json:"d"`
	ST       int64   `json:"st"`
	RST      int64   `json:"rst"`
}

// KasadaCDHeader computes the x-kpsdk-cd header locally. No client or API key is needed.
func KasadaCDHeader(call KasadaCDCall) (string, error) {
	if call.ST <= 0 || call.RST < call.ST {
		return "", errors.New("kasada: require positive ST and RST >= ST")
	}
	if call.Difficulty <= 0 || call.SubchallengeCount <= 0 {
		return "", errors.New("kasada: difficulty and subchallenge count must be positive")
	}
	platform := "tp-v2-input"
	if !call.Mobile {
		if len(call.CT) < 16 {
			return "", errors.New("kasada: desktop CT must contain at least 16 bytes")
		}
		platform += call.CT[:16]
	}
	var randomID [16]byte
	cryptoRand.Read(randomID[:])
	id := hex.EncodeToString(randomID[:])
	workTime := time.Now().UnixMilli() - int64(rand.Intn(41)+10)
	input := fmt.Sprintf("%s, %d, %s", platform, workTime, id)
	if call.CryptoCustomerKey != "" {
		input += ", " + call.CryptoCustomerKey
	}
	if call.SeedSuffix != "" {
		input += ", " + call.SeedSuffix
	}
	hash := sha256Hex(input)
	answers := make([]int64, call.SubchallengeCount)
	target := float64(call.Difficulty) / float64(call.SubchallengeCount)
	for i := range answers {
		for nonce := int64(1); ; nonce++ {
			next := sha256Hex(fmt.Sprintf("%d, %s", nonce, hash))
			prefix, _ := strconv.ParseUint(next[:13], 16, 64)
			if float64(0x10000000000000)/float64(prefix+1) >= target {
				answers[i] = nonce
				hash = next
				break
			}
		}
	}
	duration := math.Round((0.5+rand.Float64()*1.5)*10) / 10
	var data []byte
	var err error
	if call.Mobile {
		data, err = json.Marshal(cdMobile{workTime, call.RST, duration, id, call.ST, answers, call.RST - call.ST})
	} else {
		data, err = json.Marshal(cdDesktop{workTime, id, answers, duration, call.RST - call.ST, call.ST, call.RST})
	}
	return string(data), err
}

// CDHeader builds the desktop x-kpsdk-cd header from the /tl response's
// x-kpsdk-ct value and x-kpsdk-st header (ms). Use KasadaCDHeader for mobile.
func (p *KasadaPScriptResponse) CDHeader(ct string, st int64) (string, error) {
	return KasadaCDHeader(KasadaCDCall{
		ST: st, RST: max(time.Now().UnixMilli(), st), CT: ct,
		CryptoCustomerKey: p.CryptoCustomerKey, SeedSuffix: p.SeedSuffix,
		Difficulty: p.Difficulty, SubchallengeCount: p.SubchallengeCount,
	})
}

func sha256Hex(value string) string {
	hash := sha256.Sum256([]byte(value))
	return hex.EncodeToString(hash[:])
}

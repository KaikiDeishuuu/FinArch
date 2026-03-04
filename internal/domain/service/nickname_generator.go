package service

import (
	"crypto/sha1"
	"encoding/hex"
	"math/rand"
	"strings"
	"time"
)

const nicknameMaxLen = 24

var nicknameFirst = []string{
	"Prime", "Nova", "Silent", "Zenith", "Lunar", "Apex", "Vector", "Summit", "Core", "Urban", "Modern", "Steady", "Clear", "Solid", "Swift",
}

var nicknameSecond = []string{
	"Ledger", "Index", "Atlas", "Flow", "Signal", "Bridge", "Matrix", "Point", "Scope", "Line", "Frame", "Beacon", "Anchor", "Metric", "Harbor",
}

func generateProfessionalNickname(seed string, attempt int) string {
	h := sha1.Sum([]byte(seed + ":" + string(rune(attempt+65))))
	n := rand.New(rand.NewSource(int64(h[0])<<24 | int64(h[1])<<16 | int64(h[2])<<8 | int64(h[3])))
	first := nicknameFirst[n.Intn(len(nicknameFirst))]
	second := nicknameSecond[n.Intn(len(nicknameSecond))]
	name := first + " " + second
	if attempt > 0 {
		suffix := strings.ToUpper(hex.EncodeToString(h[:]))[:3]
		name += "-" + suffix
	}
	if len(name) > nicknameMaxLen {
		name = name[:nicknameMaxLen]
	}
	return strings.TrimSpace(name)
}

func randomNickname() string {
	seed := time.Now().Format(time.RFC3339Nano)
	return generateProfessionalNickname(seed, 0)
}

// GenerateNicknameForTest exposes deterministic generation for tests.
func GenerateNicknameForTest(seed string, attempt int) string {
	return generateProfessionalNickname(seed, attempt)
}

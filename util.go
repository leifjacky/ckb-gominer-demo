package main

import (
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"math"
	"math/big"
	"strconv"
	"time"
)

func UInt64BEToBytes(i uint64) []byte {
	b := make([]byte, 8)
	binary.BigEndian.PutUint64(b, i)
	return b
}

func MustStringToHexBytes(st string) []byte {
	b, _ := hex.DecodeString(st)
	return b
}

func Hash2BigTarget(hash []byte) *big.Int {
	return new(big.Int).SetBytes(hash[:])
}

func MustParseInt64(str string, base int) int64 {
	i, _ := strconv.ParseInt(str, base, 64)
	return i
}

func MustParseDuration(s string) time.Duration {
	value, err := time.ParseDuration(s)
	if err != nil {
		panic("util: Can't parse duration `" + s + "`: " + err.Error())
	}
	return value
}

func GetReadableHashRateString(hashrate float64) string {
	if hashrate <= 0 {
		return "0 " + "H"
	}

	units := []string{"H", "K", "M", "G", "T", "P", "E", "Z", "Y"}

	i := int64(math.Min(float64(len(units)-1), math.Max(0.0, math.Floor(math.Log(hashrate)/math.Log(1000.0)))))
	hr_float := hashrate / math.Pow(1000.0, float64(i))

	return fmt.Sprintf("%.3f %s", hr_float, units[i])
}

func FillZeroHashLen(hash string, l int) string {
	for len(hash) < l {
		hash = "0" + hash
	}
	return hash
}

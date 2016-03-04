package pds

import (
	"fmt"
	"hash/fnv"
	"math"
)

const (
	byteSize      = 8
	bytesIn32Bits = 4
)

// hash takes a string and hashes it into a uint32
func hash(value string) uint32 {
	h := fnv.New32a()
	h.Write([]byte(value))

	return h.Sum32()
}

// bucket contains the cardinality estimate
type bucket struct {
	cardinalityEstimation int
}

// findRun figures out how many zeros in a row from the first item there is
// eg. 00011111000101 returns 3
func findRun(a uint32) int {
	return int(math.Log2(float64(a & -a)))
}

// updateLongestRun will update a bucket if a longer zero run has been found
func (b *bucket) updateLongestRun(value uint32) {
	cardinalityEstimation := findRun(value) + 1

	if b.cardinalityEstimation < cardinalityEstimation {
		b.cardinalityEstimation = cardinalityEstimation
	}
}

type bucketGroup []bucket

// newBucketGroup creates a new bucket group
func newBucketGroup(mBuckets int64) bucketGroup {
	return make(bucketGroup, mBuckets)
}

// Len returns the number of buckets in the group
func (bg bucketGroup) Len() float64 {
	return float64(len(bg))
}

// countZeroBuckets returns the number of buckets that have nothing in them
func (bg bucketGroup) countZeroBuckets() float64 {
	var count float64
	for _, bucket := range bg {
		if bucket.cardinalityEstimation == 0 {
			count++
		}
	}

	return count
}

// smallRangeCorrection returns a better cardinality estimate for smaller sets
func (bg bucketGroup) smallRangeCorrection(prediction float64, totalBuckets float64) float64 {
	return totalBuckets * math.Log(totalBuckets/bg.countZeroBuckets())
}

// correct will return a better cardinality prediction if the set is too small
func (bg bucketGroup) correct(prediction float64) float64 {

	switch {
	case prediction <= 2.5*bg.Len():
		return bg.smallRangeCorrection(prediction, bg.Len())
	default:
		return prediction
	}
}

// harmonicMean calculates a mean of some group, reducing the impact of extreme values
func (bg bucketGroup) harmonicMean(constant float64) int64 {
	var total float64
	for _, v := range bg {
		total += math.Pow(2, float64(-1*v.cardinalityEstimation))
	}

	return int64(bg.correct((constant * bg.Len() * bg.Len()) / total))
}

// HyperLogLog for estimating the cardinality of massive sets
type HyperLogLog struct {
	constant    float64
	indexBits   uint32
	mBuckets    int64
	bucketGroup bucketGroup
}

// NewHyperLogLog builds a new HyperLogLog struct
func NewHyperLogLog(indexBits uint32) (HyperLogLog, error) {

	if indexBits < 4 || indexBits > 16 {
		return HyperLogLog{}, fmt.Errorf("index bits need to be in interval 4>=x>=16")
	}

	mBuckets := math.Pow(2, float64(indexBits))

	var constant float64
	switch indexBits {
	case 4:
		constant = 0.673
	case 5:
		constant = 0.697
	case 6:
		constant = 0.709
	default:
		constant = 0.7213 / (1 + 1.079/mBuckets)
	}

	return HyperLogLog{
		constant:    constant,
		indexBits:   uint32(indexBits),
		mBuckets:    int64(mBuckets),
		bucketGroup: newBucketGroup(int64(mBuckets)),
	}, nil
}

// getHeadBitTotal gets the numeric value from a byte
func getHeadBitTotal(bits uint32, byteNumber uint32) uint32 {
	var x uint32

	start := byteSize*byteNumber - byteSize
	for i := start; i < bits+start; i++ {
		x += uint32(math.Pow(float64(2), float64(i)))
	}

	return x
}

// getSignificantBits gets the numeric value of n significant bits from a fully on binary
// eg. 11111111 for 4 bits would return 15
func getSignificantBits(n uint32) uint32 {
	if n <= byteSize {
		return getHeadBitTotal(n, 1)
	} else {
		remainingBits := n - byteSize
		return getHeadBitTotal(byteSize, 1) + getHeadBitTotal(remainingBits, 2)
	}
}

// splitBinary splits the given number into a part used for indexing and part used to count zeros
func (hll *HyperLogLog) splitBinary(h uint32) (uint32, uint32) {
	binaryTotal := getSignificantBits(hll.indexBits)
	// Compute AND on a all on binary to our binary to find the index
	// eg. 11111111 & 00000011 = 3
	binaryIndex := h & binaryTotal

	// Shift remaining binary for later zero counting
	unusedBinary := ((h - binaryIndex) >> hll.indexBits)

	return binaryIndex, unusedBinary
}

// Add hashes and puts some string into the data structure
func (hll *HyperLogLog) Add(s string) {
	h := hash(s)
	binaryIndex, unusedBinary := hll.splitBinary(h)
	hll.bucketGroup[binaryIndex].updateLongestRun(unusedBinary)
}

// EstimateCardinality returns the current hyper log log cardinality estimate
func (hll *HyperLogLog) EstimateCardinality() int64 {
	return hll.bucketGroup.harmonicMean(hll.constant)
}

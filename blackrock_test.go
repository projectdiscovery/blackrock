package blackrock

import (
	"math"
	"math/rand"
	"reflect"
	"testing"
)

func TestNew(t *testing.T) {
	type args struct {
		rangez int64
		seed   int64
	}
	tests := []struct {
		name string
		args func(t *testing.T) args

		want1 *BlackRock
	}{
		{
			name: "must solve the square root and increment B while the result is less than rangez",
			args: func(*testing.T) args {
				return args{
					rangez: 4,
					seed:   1,
				}
			},
			want1: &BlackRock{
				Rounds: 3,
				Seed:   1,
				Range:  4,
				A:      1,
				B:      5,
			},
		},
		{
			name: "if split is zero the value of A must be 1",
			args: func(*testing.T) args {
				return args{
					rangez: 0,
					seed:   1,
				}
			},
			want1: &BlackRock{
				Rounds: 3,
				Seed:   1,
				Range:  0,
				A:      1,
				B:      1,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tArgs := tt.args(t)

			got1 := New(tArgs.rangez, tArgs.seed)

			if !reflect.DeepEqual(got1, tt.want1) {
				t.Errorf("NewBlackRock got1 = %v, want1: %v", got1, tt.want1)
			}
		})
	}
}

var testSeeds = []int64{0, 1, 42, 12345}

// UnShuffle must invert Shuffle over the whole domain. This is the property the
// caller relies on to map a shuffled index back to the original one.
func TestShuffleUnShuffleRoundTrip(t *testing.T) {
	tests := []struct {
		name    string
		rangez  int64
		samples int64 // 0 means walk the whole range
	}{
		{name: "tiny range", rangez: 1},
		{name: "range smaller than the split", rangez: 4},
		{name: "small range", rangez: 1000},
		{name: "medium range", rangez: 100000, samples: 20000},
		{name: "ipv4 sized range", rangez: 651399200, samples: 20000},
		{name: "range wider than 32 bits", rangez: 6300000000, samples: 20000},
	}

	for _, tt := range tests {
		for _, seed := range testSeeds {
			t.Run(tt.name, func(t *testing.T) {
				blackrock := New(tt.rangez, seed)

				step := int64(1)
				if tt.samples > 0 && tt.rangez > tt.samples {
					step = tt.rangez / tt.samples
				}

				for i := int64(0); i < tt.rangez; i += step {
					shuffled := blackrock.Shuffle(i)
					if got := blackrock.UnShuffle(shuffled); got != i {
						t.Fatalf("UnShuffle(Shuffle(%d)) = %d, want %d (range %d, seed %d, shuffled %d)",
							i, got, i, tt.rangez, seed, shuffled)
					}
				}
			})
		}
	}
}

// Shuffle must be a permutation of [0, Range): every input maps to a distinct
// output that stays inside the range.
func TestShuffleIsPermutation(t *testing.T) {
	for _, rangez := range []int64{1, 4, 1000, 65536} {
		for _, seed := range testSeeds {
			blackrock := New(rangez, seed)
			seen := make([]bool, rangez)

			for i := int64(0); i < rangez; i++ {
				shuffled := blackrock.Shuffle(i)

				if shuffled < 0 || shuffled >= rangez {
					t.Fatalf("Shuffle(%d) = %d, out of range [0, %d) (seed %d)", i, shuffled, rangez, seed)
				}
				if seen[shuffled] {
					t.Fatalf("Shuffle(%d) = %d, already produced by an earlier input (range %d, seed %d)",
						i, shuffled, rangez, seed)
				}

				seen[shuffled] = true
			}
		}
	}
}

// Unfe must invert Fe for both an odd and an even number of rounds. Each parity
// exercises a different branch of the reverse construction, so an error in one
// of them stays invisible when only the default round count is tested.
func TestUnfeInvertsFe(t *testing.T) {
	blackrock := New(100000, 7)

	for _, rounds := range []int64{1, 2, 3} {
		for _, seed := range testSeeds {
			for m := int64(0); m < 5000; m++ {
				encrypted := blackrock.Fe(rounds, blackrock.A, blackrock.B, m, seed)
				if got := blackrock.Unfe(rounds, blackrock.A, blackrock.B, encrypted, seed); got != m {
					t.Fatalf("Unfe(Fe(%d)) = %d, want %d (rounds %d, seed %d)", m, got, m, rounds, seed)
				}
			}
		}
	}
}

// The reverse construction subtracts R from the round function output before
// wrapping it. Getting that sign wrong only shows up when F(j, L, seed) exceeds
// R, which is the common case but was never covered by a test.
func TestUnfeSubtractsRoundOutput(t *testing.T) {
	const (
		rounds = 3
		seed   = 1
	)

	blackrock := New(1000, seed)
	exercised := false

	for m := int64(0); m < 1000; m++ {
		L := m % blackrock.A
		R := m / blackrock.A
		if blackrock.F(rounds, L, seed) > R {
			exercised = true
		}

		encrypted := blackrock.Fe(rounds, blackrock.A, blackrock.B, m, seed)
		if got := blackrock.Unfe(rounds, blackrock.A, blackrock.B, encrypted, seed); got != m {
			t.Fatalf("Unfe(Fe(%d)) = %d, want %d", m, got, m)
		}
	}

	if !exercised {
		t.Fatal("no input reached the F(j, L, seed) > R branch, the test no longer covers it")
	}
}

// testPrimes mirrors the table F builds internally. Keeping a copy here lets the
// tests below recompute what F is supposed to return without exporting it.
var testPrimes = []int64{961752031, 982324657, 15485843, 961752031}

// fReference recomputes F in unsigned arithmetic, where wrapping is defined by
// the spec rather than by the width of a mantissa. Signed and unsigned +, *, ^
// and << agree bit for bit on two's complement, so this is the exact value F
// must produce once the sign bit is cleared.
func fReference(j, r, seed int64) int64 {
	u := uint64(r)
	u = (u << (u & 0x4)) + u + uint64(seed)
	v := ((uint64(testPrimes[j])*u + 25) ^ u) + uint64(j)

	return int64(v & math.MaxInt64)
}

// F must return the exact integer result, not one that has been rounded off by
// a trip through float64. Only values above 2^53 can catch this: below that the
// mantissa still holds every bit, so a rounding implementation passes.
func TestFKeepsFullPrecision(t *testing.T) {
	rnd := rand.New(rand.NewSource(1))
	blackrock := New(1000, 1)

	aboveMantissa := 0
	for i := 0; i < 200000; i++ {
		j := int64(i%3) + 1
		r := rnd.Int63()
		seed := testSeeds[i%len(testSeeds)]

		want := fReference(j, r, seed)
		if got := blackrock.F(j, r, seed); got != want {
			t.Fatalf("F(%d, %d, %d) = %d, want %d", j, r, seed, got, want)
		}
		if want > 1<<53 {
			aboveMantissa++
		}
	}

	if aboveMantissa == 0 {
		t.Fatal("no result exceeded 2^53, the test no longer covers the rounding case")
	}
}

// Callers feed F straight into (L + F) % a, so a negative result would push the
// whole round out of [0, a) and break the permutation. The raw expression F is
// built from is negative about half the time, which is exactly what the sign bit
// mask has to absorb.
func TestFIsNeverNegative(t *testing.T) {
	rnd := rand.New(rand.NewSource(2))
	blackrock := New(1000, 1)

	rawNegative := 0
	for i := 0; i < 200000; i++ {
		j := int64(i%3) + 1
		r := rnd.Int63()
		if i%2 == 0 {
			r = -r
		}
		seed := testSeeds[i%len(testSeeds)]

		got := blackrock.F(j, r, seed)
		if got < 0 {
			t.Fatalf("F(%d, %d, %d) = %d, want a non-negative result", j, r, seed, got)
		}

		// The unmasked expression, to confirm the mask is doing real work.
		u := uint64(r)
		u = (u << (u & 0x4)) + u + uint64(seed)
		if int64(((uint64(testPrimes[j])*u+25)^u)+uint64(j)) < 0 {
			rawNegative++
		}
	}

	if rawNegative == 0 {
		t.Fatal("no input produced a negative raw value, the test no longer covers the mask")
	}
}

// maxWorkingRange is the largest range New can build a split for. Above it
// A*B overflows int64 and the loop in New never terminates, so the tests stop
// here on purpose.
const maxWorkingRange = 9223372033963249497

// Ranges past 2^53 are where a rounded round function would start handing back
// values that no longer round trip. The domains are far too large to walk, so
// each one is sampled, and both directions are checked: Shuffle must land inside
// the range and UnShuffle must take it back, and the same in reverse.
func TestShuffleUnShuffleLargeRanges(t *testing.T) {
	tests := []struct {
		name   string
		rangez int64
	}{
		{name: "2^40", rangez: 1 << 40},
		{name: "2^53, the float64 mantissa limit", rangez: 1 << 53},
		{name: "just past 2^53", rangez: 1<<53 + 12345},
		{name: "2^62", rangez: 1 << 62},
		{name: "5e18", rangez: 5000000000000000000},
		{name: "largest range New can split", rangez: maxWorkingRange},
	}

	const samples = 2000

	for _, tt := range tests {
		for _, seed := range testSeeds {
			t.Run(tt.name, func(t *testing.T) {
				blackrock := New(tt.rangez, seed)
				rnd := rand.New(rand.NewSource(seed))

				for i := 0; i < samples; i++ {
					m := rnd.Int63n(tt.rangez)
					switch i {
					case 0:
						m = 0
					case 1:
						m = tt.rangez - 1
					}

					shuffled := blackrock.Shuffle(m)
					if shuffled < 0 || shuffled >= tt.rangez {
						t.Fatalf("Shuffle(%d) = %d, out of range [0, %d) (seed %d)", m, shuffled, tt.rangez, seed)
					}
					if got := blackrock.UnShuffle(shuffled); got != m {
						t.Fatalf("UnShuffle(Shuffle(%d)) = %d, want %d (range %d, seed %d)",
							m, got, m, tt.rangez, seed)
					}

					// UnShuffle is reachable on its own, so it has to invert in
					// the other direction as well.
					unshuffled := blackrock.UnShuffle(m)
					if unshuffled < 0 || unshuffled >= tt.rangez {
						t.Fatalf("UnShuffle(%d) = %d, out of range [0, %d) (seed %d)", m, unshuffled, tt.rangez, seed)
					}
					if got := blackrock.Shuffle(unshuffled); got != m {
						t.Fatalf("Shuffle(UnShuffle(%d)) = %d, want %d (range %d, seed %d)",
							m, got, m, tt.rangez, seed)
					}
				}
			})
		}
	}
}

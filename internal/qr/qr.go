// Package qr makes QR codes (ISO/IEC 18004) for text such as a pairing
// address, and draws them in a terminal (D104, T109.1): byte mode, error
// correction level M, versions 1 to 10 (up to 213 bytes), the mask with the
// lowest penalty. The construction follows the standard's tables; no
// module outside the standard library is used (D13).
package qr

import (
	"errors"
	"strings"
)

// Code is a QR code's modules: Size × Size, true for dark.
type Code struct {
	Size    int
	Version int
	Mask    int
	dark    [][]bool
	fn      [][]bool // function modules (finders, timing, alignment, format, version)
}

// Dark reports whether the module at column x, row y is dark.
func (c *Code) Dark(x, y int) bool { return c.dark[y][x] }

// ErrTooLong means the text does not fit in version 10 at level M.
var ErrTooLong = errors.New("qr: text too long")

// Level M's blocks for versions 1..10: error correction codewords per
// block and the number of blocks (the standard's Table 9).
var (
	eccPerBlock = [11]int{0, 10, 16, 26, 18, 24, 16, 18, 22, 22, 26}
	numBlocks   = [11]int{0, 1, 1, 1, 2, 2, 4, 4, 4, 5, 5}
)

// alignment are the alignment patterns' centers for versions 1..10.
var alignment = [11][]int{
	nil, nil,
	{6, 18},
	{6, 22},
	{6, 26},
	{6, 30},
	{6, 34},
	{6, 22, 38},
	{6, 24, 42},
	{6, 26, 46},
	{6, 28, 50},
}

// formatM is level M's two format bits.
const formatM = 0

// rawModules is how many modules of version v carry data and error
// correction (everything but the function patterns).
func rawModules(v int) int {
	n := (16*v+128)*v + 64
	if v >= 2 {
		a := v/7 + 2
		n -= (25*a-10)*a - 55
		if v >= 7 {
			n -= 36
		}
	}
	return n
}

// dataCodewords is how many data codewords version v holds at level M.
func dataCodewords(v int) int {
	return rawModules(v)/8 - eccPerBlock[v]*numBlocks[v]
}

// Encode makes the smallest QR code (version 1..10, level M, byte mode)
// that holds text.
func Encode(text string) (*Code, error) {
	data := []byte(text)
	v := 0
	for ver := 1; ver <= 10; ver++ {
		countBits := 8
		if ver >= 10 {
			countBits = 16
		}
		if 4+countBits+8*len(data) <= dataCodewords(ver)*8 {
			v = ver
			break
		}
	}
	if v == 0 {
		return nil, ErrTooLong
	}
	cw := codewords(data, v)
	all := withECC(cw, v)

	best := (*Code)(nil)
	bestScore := 0
	for mask := 0; mask < 8; mask++ {
		c := newCode(v)
		c.placeData(all)
		c.applyMask(mask)
		c.drawFormat(mask)
		if s := c.penalty(); best == nil || s < bestScore {
			best, bestScore = c, s
		}
	}
	return best, nil
}

// codewords is the data bit stream of byte mode, terminated and padded to
// version v's data capacity.
func codewords(data []byte, v int) []byte {
	var bits []bool
	put := func(val, n int) {
		for i := n - 1; i >= 0; i-- {
			bits = append(bits, (val>>i)&1 == 1)
		}
	}
	put(0b0100, 4) // byte mode
	if v >= 10 {
		put(len(data), 16)
	} else {
		put(len(data), 8)
	}
	for _, b := range data {
		put(int(b), 8)
	}
	capBits := dataCodewords(v) * 8
	put(0, min(4, capBits-len(bits))) // terminator
	for len(bits)%8 != 0 {
		bits = append(bits, false)
	}
	out := make([]byte, 0, dataCodewords(v))
	for i := 0; i < len(bits); i += 8 {
		var b byte
		for j := range 8 {
			if bits[i+j] {
				b |= 1 << (7 - j)
			}
		}
		out = append(out, b)
	}
	for pad := byte(0xEC); len(out) < dataCodewords(v); pad ^= 0xEC ^ 0x11 {
		out = append(out, pad)
	}
	return out
}

// withECC splits data into version v's blocks, adds each one's error
// correction and interleaves them.
func withECC(data []byte, v int) []byte {
	nb, ecLen := numBlocks[v], eccPerBlock[v]
	raw := rawModules(v) / 8
	numShort := nb - raw%nb
	shortLen := raw / nb
	div := rsDivisor(ecLen)
	blocks := make([][]byte, nb)
	k := 0
	for i := range nb {
		n := shortLen - ecLen
		if i >= numShort {
			n++
		}
		dat := append([]byte(nil), data[k:k+n]...)
		k += n
		ecc := rsRemainder(dat, div)
		if i < numShort {
			dat = append(dat, 0) // a gap, skipped when interleaving
		}
		blocks[i] = append(dat, ecc...)
	}
	var out []byte
	for i := range blocks[0] {
		for j, b := range blocks {
			if i != shortLen-ecLen || j >= numShort {
				out = append(out, b[i])
			}
		}
	}
	return out
}

// gfMul multiplies in GF(2^8) modulo x^8 + x^4 + x^3 + x^2 + 1.
func gfMul(x, y byte) byte {
	var z int
	for i := 7; i >= 0; i-- {
		z = (z << 1) ^ ((z >> 7) * 0x11D)
		z ^= int((y>>i)&1) * int(x)
	}
	return byte(z)
}

// rsDivisor is the Reed–Solomon generator polynomial of degree n (its
// coefficients, highest first, without the leading 1).
func rsDivisor(n int) []byte {
	res := make([]byte, n)
	res[n-1] = 1
	root := byte(1)
	for range n {
		for j := range res {
			res[j] = gfMul(res[j], root)
			if j+1 < len(res) {
				res[j] ^= res[j+1]
			}
		}
		root = gfMul(root, 2)
	}
	return res
}

// rsRemainder is data's Reed–Solomon error correction for divisor.
func rsRemainder(data, div []byte) []byte {
	res := make([]byte, len(div))
	for _, b := range data {
		f := b ^ res[0]
		copy(res, res[1:])
		res[len(res)-1] = 0
		for i := range res {
			res[i] ^= gfMul(div[i], f)
		}
	}
	return res
}

// newCode is version v's function patterns, the format left for later.
func newCode(v int) *Code {
	size := 4*v + 17
	c := &Code{Size: size, Version: v}
	c.dark = make([][]bool, size)
	c.fn = make([][]bool, size)
	for i := range size {
		c.dark[i] = make([]bool, size)
		c.fn[i] = make([]bool, size)
	}
	for i := range size { // timing
		c.set(6, i, i%2 == 0)
		c.set(i, 6, i%2 == 0)
	}
	for _, p := range [][2]int{{3, 3}, {size - 4, 3}, {3, size - 4}} { // finders and separators
		for dy := -4; dy <= 4; dy++ {
			for dx := -4; dx <= 4; dx++ {
				x, y := p[0]+dx, p[1]+dy
				if x < 0 || x >= size || y < 0 || y >= size {
					continue
				}
				d := max(abs(dx), abs(dy))
				c.set(x, y, d != 2 && d != 4)
			}
		}
	}
	al := alignment[v]
	for i, ay := range al {
		for j, ax := range al {
			if (i == 0 && j == 0) || (i == 0 && j == len(al)-1) || (i == len(al)-1 && j == 0) {
				continue // under a finder
			}
			for dy := -2; dy <= 2; dy++ {
				for dx := -2; dx <= 2; dx++ {
					c.set(ax+dx, ay+dy, max(abs(dx), abs(dy)) != 1)
				}
			}
		}
	}
	c.drawFormat(0) // reserves the format modules; drawn again with the mask
	if v >= 7 {
		rem := v
		for range 12 {
			rem = (rem << 1) ^ ((rem >> 11) * 0x1F25)
		}
		bits := v<<12 | rem
		for i := range 18 {
			bit := (bits>>i)&1 == 1
			a, b := size-11+i%3, i/3
			c.set(a, b, bit)
			c.set(b, a, bit)
		}
	}
	return c
}

func abs(x int) int {
	if x < 0 {
		return -x
	}
	return x
}

// set makes the module at column x, row y a function module.
func (c *Code) set(x, y int, dark bool) {
	c.dark[y][x] = dark
	c.fn[y][x] = true
}

// drawFormat writes level M's and mask's format information, both copies,
// and the dark module.
func (c *Code) drawFormat(mask int) {
	data := formatM<<3 | mask
	rem := data
	for range 10 {
		rem = (rem << 1) ^ ((rem >> 9) * 0x537)
	}
	bits := (data<<10 | rem) ^ 0x5412
	bit := func(i int) bool { return (bits>>i)&1 == 1 }
	for i := 0; i <= 5; i++ {
		c.set(8, i, bit(i))
	}
	c.set(8, 7, bit(6))
	c.set(8, 8, bit(7))
	c.set(7, 8, bit(8))
	for i := 9; i < 15; i++ {
		c.set(14-i, 8, bit(i))
	}
	for i := range 8 {
		c.set(c.Size-1-i, 8, bit(i))
	}
	for i := 8; i < 15; i++ {
		c.set(8, c.Size-15+i, bit(i))
	}
	c.set(8, c.Size-8, true)
}

// placeData lays the codewords out in the standard's zigzag.
func (c *Code) placeData(data []byte) {
	i := 0
	for right := c.Size - 1; right >= 1; right -= 2 {
		if right == 6 {
			right = 5 // the vertical timing column
		}
		for vert := range c.Size {
			for j := range 2 {
				x := right - j
				y := vert
				if (right+1)&2 == 0 {
					y = c.Size - 1 - vert
				}
				if !c.fn[y][x] && i < len(data)*8 {
					c.dark[y][x] = (data[i>>3]>>(7-(i&7)))&1 == 1
					i++
				}
			}
		}
	}
}

// applyMask flips the data modules where mask's condition holds.
func (c *Code) applyMask(mask int) {
	c.Mask = mask
	for y := range c.Size {
		for x := range c.Size {
			if c.fn[y][x] {
				continue
			}
			var flip bool
			switch mask {
			case 0:
				flip = (x+y)%2 == 0
			case 1:
				flip = y%2 == 0
			case 2:
				flip = x%3 == 0
			case 3:
				flip = (x+y)%3 == 0
			case 4:
				flip = (x/3+y/2)%2 == 0
			case 5:
				flip = x*y%2+x*y%3 == 0
			case 6:
				flip = (x*y%2+x*y%3)%2 == 0
			case 7:
				flip = ((x+y)%2+x*y%3)%2 == 0
			}
			if flip {
				c.dark[y][x] = !c.dark[y][x]
			}
		}
	}
}

// penalty is the standard's mask penalty: runs, 2×2 blocks, finder-like
// patterns and the dark balance.
func (c *Code) penalty() int {
	n := c.Size
	score := 0
	line := func(get func(i int) bool) {
		run, prev := 0, false
		for i := range n {
			d := get(i)
			if i > 0 && d == prev {
				run++
			} else {
				if run >= 5 {
					score += 3 + run - 5
				}
				run = 1
			}
			prev = d
		}
		if run >= 5 {
			score += 3 + run - 5
		}
		// 1:1:3:1:1 with four light modules on either side.
		for i := 0; i+11 <= n; i++ {
			a := [11]bool{}
			for k := range 11 {
				a[k] = get(i + k)
			}
			p1 := a[0] && !a[1] && a[2] && a[3] && a[4] && !a[5] && a[6] && !a[7] && !a[8] && !a[9] && !a[10]
			p2 := !a[0] && !a[1] && !a[2] && !a[3] && a[4] && !a[5] && a[6] && a[7] && a[8] && !a[9] && a[10]
			if p1 || p2 {
				score += 40
			}
		}
	}
	for y := range n {
		line(func(i int) bool { return c.dark[y][i] })
	}
	for x := range n {
		line(func(i int) bool { return c.dark[i][x] })
	}
	dark := 0
	for y := range n {
		for x := range n {
			if c.dark[y][x] {
				dark++
			}
			if x+1 < n && y+1 < n {
				d := c.dark[y][x]
				if c.dark[y][x+1] == d && c.dark[y+1][x] == d && c.dark[y+1][x+1] == d {
					score += 3
				}
			}
		}
	}
	total := n * n
	k := (abs(dark*20-total*10)+total-1)/total - 1
	score += k * 10
	return score
}

// Quiet is the light border around a code, in modules.
const Quiet = 4

// Terminal draws c for a terminal: two modules per character with upper
// half blocks, black on white (with 256-color escapes, so a dark or light
// terminal theme does not invert it), Quiet modules of white around it.
func (c *Code) Terminal() string {
	n := c.Size + 2*Quiet
	at := func(x, y int) bool {
		x, y = x-Quiet, y-Quiet
		return x >= 0 && y >= 0 && x < c.Size && y < c.Size && c.dark[y][x]
	}
	col := func(dark bool) string {
		if dark {
			return "16"
		}
		return "231"
	}
	var b strings.Builder
	for y := 0; y < n; y += 2 {
		for x := range n {
			top := at(x, y)
			bottom := y+1 < n && at(x, y+1)
			b.WriteString("\x1b[38;5;" + col(top) + ";48;5;" + col(bottom) + "m▀")
		}
		b.WriteString("\x1b[0m\n")
	}
	return b.String()
}

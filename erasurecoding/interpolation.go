// interpolation.go
package erasurecoding

// Add returns the sum of two GFPoints
func polyAdd(p, q []GFPoint) []GFPoint {
	if len(q) > len(p) {
		p, q = q, p
	}
	res := make([]GFPoint, len(p))
	copy(res, p)
	for i := 0; i < len(q); i++ {
		res[i] = Add(res[i], q[i])
	}
	return trimPoly(res)
}

// Mul multiplies two field elements in the Galois Field
func polyMul(p, q []GFPoint) []GFPoint {
	rlen := len(p) + len(q) - 1
	res := make([]GFPoint, rlen)
	for i := 0; i < len(p); i++ {
		for j := 0; j < len(q); j++ {
			res[i+j] = Add(res[i+j], Mul(p[i], q[j]))
		}
	}
	return trimPoly(res)
}

// trimPoly removes trailing zeros from a polynomial
func trimPoly(p []GFPoint) []GFPoint {
	i := len(p) - 1
	for i > 0 && p[i] == 0 {
		i--
	}
	return p[:i+1]
}

// Interpolate returns the polynomial that passes through the given points
func Interpolate(xs, ys []GFPoint) []GFPoint {
	n := len(xs)
	if n == 0 {
		return []GFPoint{}
	}
	if len(ys) != n {
		panic("xs, ys length mismatch")
	}
	if n == 1 {
		return []GFPoint{ys[0]}
	}

	p := make([]GFPoint, n)
	for i := 0; i < n; i++ {
		denom := GFPoint(1)
		for j := 0; j < n; j++ {
			if j == i {
				continue
			}
			diff := Add(xs[i], xs[j])
			denom = Mul(denom, diff)
		}
		invDenom := Inv(denom)

		li := []GFPoint{1}
		for j := 0; j < n; j++ {
			if j == i {
				continue
			}
			factor := []GFPoint{xs[j], 1}
			li = polyMul(li, factor)
		}

		scale := Mul(ys[i], invDenom)
		for k := range li {
			li[k] = Mul(li[k], scale)
		}

		p = polyAdd(p, li)
	}

	return p
}

// Evaluate returns the value of the polynomial at the given point
func Evaluate(p []GFPoint, x GFPoint) GFPoint {
	result := GFPoint(0)
	power := GFPoint(1)
	for i := 0; i < len(p); i++ {
		term := Mul(p[i], power)
		result = Add(result, term)
		power = Mul(power, x)
	}
	return result
}

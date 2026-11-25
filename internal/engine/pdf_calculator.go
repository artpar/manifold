package engine

import (
	"fmt"
	"math"
	"strconv"
	"strings"
)

// Bounded numeric subset of ISO 32000 calculator functions. Unimplemented
// operators reject explicitly; this does not execute a PostScript interpreter.
func pdfCalculatorTint(program []byte, input float64, outputs int) ([]float64, error) {
	source := strings.TrimSpace(string(program))
	if len(source) > 65536 || len(source) < 2 || source[0] != '{' || source[len(source)-1] != '}' {
		return nil, fmt.Errorf("invalid calculator function")
	}
	tokens := strings.Fields(source[1 : len(source)-1])
	if len(tokens) > 4096 {
		return nil, fmt.Errorf("calculator operation budget exceeded")
	}
	stack := []float64{input}
	pop := func() (float64, error) {
		if len(stack) == 0 {
			return 0, fmt.Errorf("calculator stack underflow")
		}
		v := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		return v, nil
	}
	integer := func(v float64) bool { return finite(v) && v == math.Trunc(v) && math.Abs(v) < 1<<30 }
	for _, token := range tokens {
		if v, e := strconv.ParseFloat(token, 64); e == nil {
			if !finite(v) {
				return nil, fmt.Errorf("nonfinite calculator number")
			}
			stack = append(stack, v)
		} else {
			switch token {
			case "cvr":
				if len(stack) == 0 {
					return nil, fmt.Errorf("calculator cvr stack underflow")
				}
			case "pop":
				if _, e := pop(); e != nil {
					return nil, e
				}
			case "dup":
				v, e := pop()
				if e != nil {
					return nil, e
				}
				stack = append(stack, v, v)
			case "exch":
				if len(stack) < 2 {
					return nil, fmt.Errorf("calculator exch stack underflow")
				}
				n := len(stack)
				stack[n-1], stack[n-2] = stack[n-2], stack[n-1]
			case "index":
				v, e := pop()
				if e != nil || !integer(v) || v < 0 || v >= float64(len(stack)) {
					return nil, fmt.Errorf("invalid calculator index")
				}
				stack = append(stack, stack[len(stack)-1-int(v)])
			case "roll":
				j, e := pop()
				if e != nil {
					return nil, e
				}
				n, e := pop()
				if e != nil || !integer(j) || !integer(n) || n < 0 || n > float64(len(stack)) {
					return nil, fmt.Errorf("invalid calculator roll")
				}
				if n > 0 {
					count := int(n)
					shift := int(j) % count
					if shift < 0 {
						shift += count
					}
					start := len(stack) - count
					copyOf := append([]float64(nil), stack[start:]...)
					for i, v := range copyOf {
						stack[start+(i+shift)%count] = v
					}
				}
			case "add", "sub", "mul", "div":
				b, e := pop()
				if e != nil {
					return nil, e
				}
				a, e := pop()
				if e != nil {
					return nil, e
				}
				v := a + b
				switch token {
				case "sub":
					v = a - b
				case "mul":
					v = a * b
				case "div":
					v = a / b
				}
				if !finite(v) {
					return nil, fmt.Errorf("undefined calculator arithmetic")
				}
				stack = append(stack, v)
			case "neg", "abs", "sqrt", "cvi":
				v, e := pop()
				if e != nil {
					return nil, e
				}
				switch token {
				case "neg":
					v = -v
				case "abs":
					v = math.Abs(v)
				case "sqrt":
					v = math.Sqrt(v)
				case "cvi":
					v = math.Trunc(v)
				}
				if !finite(v) {
					return nil, fmt.Errorf("undefined calculator arithmetic")
				}
				stack = append(stack, v)
			default:
				return nil, fmt.Errorf("%w: calculator operator %q", ErrUnsupported, token)
			}
		}
		if len(stack) > 100 {
			return nil, fmt.Errorf("calculator stack budget exceeded")
		}
	}
	if len(stack) != outputs {
		return nil, fmt.Errorf("calculator output count %d, expected %d", len(stack), outputs)
	}
	return stack, nil
}

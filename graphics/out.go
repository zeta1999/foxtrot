package graphics

import (
	"fmt"
	"gioui.org/layout"
	"github.com/corywalker/expreduce/expreduce/atoms"
	api "github.com/corywalker/expreduce/pkg/expreduceapi"
	"math/big"
	"strings"
)

func Ex(ex api.Ex, st *Style, gtx *layout.Context) layout.Widget {
	switch ex := ex.(type) {
	case *atoms.String:
		return String(ex, st, gtx)
	case *atoms.Integer:
		return Integer(ex, st, gtx)
	case *atoms.Flt:
		return Flt(ex, st, gtx)
	case *atoms.Rational:
		return Rational(ex, st, gtx)
	case *atoms.Complex:
		return Complex(ex, st, gtx)
	case *atoms.Symbol:
		return Symbol(ex, st, gtx)
	case *atoms.Expression:
		return Expression(ex, st, gtx)
	default:
		fmt.Println("unknown expression type")
	}
	return nil
}

func String(s *atoms.String, st *Style, gtx *layout.Context) layout.Widget {
	return func() {
		l := &Tag{MaxWidth: Inf}
		l.Layout(gtx, st, s.String())
	}
}

func Integer(i *atoms.Integer, st *Style, gtx *layout.Context) layout.Widget {
	return func() {
		l := &Tag{MaxWidth: Inf}
		l.Layout(gtx, st, i.String())
	}
}

func Flt(f *atoms.Flt, st *Style, gtx *layout.Context) layout.Widget {
	return func() {
		l := &Tag{MaxWidth: Inf}
		l.Layout(gtx, st, f.StringForm(api.ToStringParams{}))
	}
}

func Complex(i *atoms.Complex, st *Style, gtx *layout.Context) layout.Widget {
	return func() {
		l := &Tag{MaxWidth: Inf}
		l.Layout(gtx, st, i.StringForm(api.ToStringParams{}))
	}
}

func Symbol(i *atoms.Symbol, st *Style, gtx *layout.Context) layout.Widget {
	return func() {
		l := &Tag{MaxWidth: Inf}
		l.Layout(gtx, st, i.StringForm(api.ToStringParams{}))
	}
}

func Expression(ex *atoms.Expression, st *Style, gtx *layout.Context) layout.Widget {
	special := drawSpecialExpression(ex, st, gtx)
	if special != nil {
		return special
	}
	return func() {
		f := layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}
		var children []layout.FlexChild
		first := f.Rigid(gtx, func() {
			l1 := &Tag{MaxWidth: Inf}
			l1.Layout(gtx, st, shortSymbolName(ex)+"[")
		})
		children = append(children, first)
		parts := Parts(ex, f, ",", st, gtx)
		children = append(children, parts...)
		last := f.Rigid(gtx, func() {
			l1 := &Tag{MaxWidth: Inf}
			l1.Layout(gtx, st, "]")
		})
		children = append(children, last)
		f.Layout(gtx, children...)
	}
}

func drawSpecialExpression(ex *atoms.Expression, st *Style, gtx *layout.Context) layout.Widget {
	switch ex.HeadStr() {
	case "System`List":
		return List(ex, st, gtx)
	case "System`Plus":
		return drawInfix(ex, "+", st, gtx)
	case "System`Minus":
		return drawInfix(ex, "-", st, gtx)
	case "System`Times":
		return drawInfix(ex, "*", st, gtx)
	case "System`Power":
		return Power(ex, st, gtx)
	}
	return nil
}

func List(ex *atoms.Expression, st *Style, gtx *layout.Context) layout.Widget {
	return func() {
		f := layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}
		var children []layout.FlexChild
		first := f.Rigid(gtx, func() {
			l1 := &Tag{MaxWidth: Inf}
			l1.Layout(gtx, st, "{")
		})
		children = append(children, first)
		parts := Parts(ex, f, ",", st, gtx)
		children = append(children, parts...)
		last := f.Rigid(gtx, func() {
			l1 := &Tag{MaxWidth: Inf}
			l1.Layout(gtx, st, "}")
		})
		children = append(children, last)
		f.Layout(gtx, children...)
	}
}

func Power(ex *atoms.Expression, st *Style, gtx *layout.Context) layout.Widget {
	if isSqrt(ex) {
		return Sqrt(ex, st, gtx)
	}
	return drawInfix(ex, "^", st, gtx)
}

var bigOne = big.NewInt(1)
var bigTwo = big.NewInt(2)

func isSqrt(ex *atoms.Expression) bool {
	if len(ex.Parts) != 3 {
		return false
	}
	r, isRational := ex.Parts[2].(*atoms.Rational)
	if !isRational {
		return false
	}
	return r.Num.Cmp(bigOne) == 0 && r.Den.Cmp(bigTwo) == 0
}

func Sqrt(ex *atoms.Expression, st *Style, gtx *layout.Context) layout.Widget {
	return func() {
		f := layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}
		c1 := f.Rigid(gtx, func() {
			l1 := &Tag{MaxWidth: Inf}
			l1.Layout(gtx, st, "√")
		})
		c2 := f.Rigid(gtx, func() {
			part := ex.Parts[1]
			w := Ex(part, st, gtx)
			w()
		})
		// TODO: Draw line above body
		f.Layout(gtx, c1, c2)
	}
}

func drawInfix(ex *atoms.Expression, operator string, st *Style, gtx *layout.Context) layout.Widget {
	return func() {
		f := layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}
		children := Parts(ex, f, operator, st, gtx)
		f.Layout(gtx, children...)
	}
}

func Parts(ex *atoms.Expression, f layout.Flex, infix string, st *Style, gtx *layout.Context) []layout.FlexChild {
	var children []layout.FlexChild
	var comma layout.FlexChild
	for _, e := range ex.Parts[1:] {
		var w layout.Widget
		switch e := e.(type) {
		case *atoms.String:
			w = String(e, st, gtx)
		case *atoms.Integer:
			w = Integer(e, st, gtx)
		case *atoms.Flt:
			w = Flt(e, st, gtx)
		case *atoms.Rational:
			w = Rational(e, st, gtx)
		case *atoms.Complex:
			w = Complex(e, st, gtx)
		case *atoms.Symbol:
			w = Symbol(e, st, gtx)
		case *atoms.Expression:
			w = Expression(e, st, gtx)
		}
		children = append(children, comma)
		comma = f.Rigid(gtx, func() {
			t := &Tag{MaxWidth: Inf}
			t.Layout(gtx, st, infix)
		})
		c := f.Rigid(gtx, w)
		children = append(children, c)
	}
	return children
}

func shortSymbolName(ex *atoms.Expression) string {
	name := ex.HeadStr()
	if strings.HasPrefix(name, "System`") {
		return name[7:]
	} else {
		return name
	}
}
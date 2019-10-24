package foxtrot

import (
	"fmt"
	"gioui.org/app"
	"gioui.org/io/system"
	"gioui.org/layout"
	"gioui.org/text"
	"gioui.org/widget"
	"gioui.org/widget/material"
	"golang.org/x/exp/shiny/materialdesign/icons"
	"log"

	_ "gioui.org/font/gofont"
	"gioui.org/unit"
	"github.com/corywalker/expreduce/expreduce"
	"github.com/corywalker/expreduce/expreduce/atoms"
	"github.com/corywalker/expreduce/expreduce/parser"
	"github.com/corywalker/expreduce/pkg/expreduceapi"
	_ "golang.org/x/image/font/gofont/gomono"
	//"golang.org/x/image/font/gofont/goregular"
	"golang.org/x/image/font/sfnt"
)

var (
	_defaultFontSize = unit.Sp(20)
	//_regularFont     = &shape.Family{Regular: mustLoadFont(goregular.TTF)}
	_monoFont = text.Font{Size: unit.Sp(20), Typeface: "Mono", Style: text.Regular}
	_blue     = rgb(0x5c6bc0)
	addIcon   *material.Icon
	theme     = material.NewTheme()
)

func init() {
	theme.TextSize = _defaultFontSize
}

func mustLoadFont(fontData []byte) *sfnt.Font {
	fnt, err := sfnt.Parse(fontData)
	if err != nil {
		panic("failed to load font")
	}
	return fnt
}

func RunUI(engine *expreduce.EvalState) {
	var err error
	addIcon, err = material.NewIcon(icons.ContentAdd)
	if err != nil {
		log.Fatal(err)
	}
	go func() {
		w := app.NewWindow(app.Title("Foxtrot"))
		a := NewApp(engine)
		if err := a.loop(w); err != nil {
			log.Fatal(err)
		}
	}()
	app.Main()
}

func (a *App) loop(w *app.Window) error {
	gtx := &layout.Context{
		Queue: w.Queue(),
	}
	a.focusNext(len(a.cells))
	for {
		e := <-w.Events()
		switch e := e.(type) {
		case system.DestroyEvent:
			return e.Err
		case system.FrameEvent:
			gtx.Reset(e.Config, e.Size)
			a.Layout(gtx)
			e.Frame(gtx.Ops)
		}
	}
}

func NewApp(engine *expreduce.EvalState) *App {
	theme := material.NewTheme()
	theme.TextSize = _defaultFontSize
	list := &layout.List{
		Axis: layout.Vertical,
	}
	editor := &widget.Editor{
		Submit: true,
	}
	entries := make([]Cell, 0)
	return &App{theme, list, editor, engine, entries, 1, newAdd()}
}

type App struct {
	theme       *material.Theme
	list        *layout.List
	editor      *widget.Editor
	engine      *expreduce.EvalState
	cells       []Cell
	promptCount int
	add         *Add
}

func (a *App) Event(gtx *layout.Context) interface{} {
	for _, e := range a.editor.Events(gtx) {
		if _, ok := e.(widget.SubmitEvent); ok {
			a.evaluate()
		}
	}
	for i, c := range a.cells {
		e := c.Event(gtx)
		if _, ok := e.(EvalEvent); ok {
			a.eval(i)
			a.focusNext(i)
		}
	}
	e := a.add.Event(gtx)
	if _, ok := e.(AddCellEvent); ok {
		a.AddCell("", "")
	}
	return nil
}

func (a *App) Layout(gtx *layout.Context) {
	a.Event(gtx)
	//a.theme.IconButton(addIcon).Layout(gtx, a.addButton)
	//ed := a.theme.Editor("Send a message")
	//ed.Font.Size = unit.Sp(14)
	//ed.Layout(gtx, a.editor)
	a.drawEntries(gtx, _defaultFontSize)
}

func (a *App) eval(i int) {
	c := &a.cells[i]
	textIn := c.inEditor.Text()
	if textIn == "" {
		return
	}
	expIn := parser.Interp(textIn, a.engine)
	expOut := a.engine.Eval(expIn)
	textOut := expressionToString(a.engine, expOut, a.promptCount)
	c.out = textOut
	c.promptNum = a.promptCount
	a.promptCount++
}

func (a *App) focusNext(i int) {
	if i < len(a.cells)-1 {
		a.cells[i+1].Focus()
	} else {
		a.add.Focus()
	}
}

func (a *App) evaluate() {
	textIn := a.editor.Text()
	if textIn == "" {
		return
	}
	expIn := parser.Interp(textIn, a.engine)
	expOut := a.engine.Eval(expIn)
	textOut := expressionToString(a.engine, expOut, a.promptCount)
	a.AddCell(textIn, textOut)
	a.editor.SetText("")
}

func (a *App) drawEntries(gtx *layout.Context, size unit.Value) {
	inset := layout.UniformInset(unit.Dp(8))
	inset.Layout(gtx, func() {
		n := len(a.cells) + 1
		a.list.Layout(gtx, n, func(i int) {
			if i == n-1 {
				a.add.Layout(gtx)
			} else {
				a.cells[i].Layout(gtx)
			}
		})
	})
}

func (a *App) AddCell(in, out string) {
	cell := newCell(in, out, -1)
	a.cells = append(a.cells, cell)
	cell.Focus()
}

func expressionToString(es *expreduce.EvalState, exp expreduceapi.Ex, promptCount int) string {
	var res string
	isNull := false
	asSym, isSym := exp.(*atoms.Symbol)
	if isSym {
		if asSym.Name == "System`Null" {
			isNull = true
		}
	}

	if !isNull {
		// Print formatted result
		specialForms := []string{
			"System`FullForm",
			"System`OutputForm",
		}
		wasSpecialForm := false
		for _, specialForm := range specialForms {
			asSpecialForm, isSpecialForm := atoms.HeadAssertion(exp, specialForm)
			if !isSpecialForm {
				continue
			}
			if len(asSpecialForm.Parts) != 2 {
				continue
			}
			res = fmt.Sprintf(
				"//%s= %s",
				specialForm[7:],
				asSpecialForm.Parts[1].StringForm(
					expreduce.ActualStringFormArgsFull(specialForm[7:], es)),
			)
			wasSpecialForm = true
		}
		if !wasSpecialForm {
			res = fmt.Sprintf("%s", exp.StringForm(expreduce.ActualStringFormArgsFull("InputForm", es)))
		}
	}
	return res
}

const gtmp = `Graphics[{{Directive[Opacity[1.], RGBColor[0.37, 0.5, 0.71], AbsoluteThickness[1.6]], Line[{{-1., 0.}, {-0.996, 0.004}, {-0.992, 0.008}, {-0.988, 0.012}, {-0.984, 0.016}, {-0.98, 0.02}, {-0.976, 0.024}, {-0.972, 0.028}, {-0.968, 0.032}, {-0.964, 0.036}, {-0.96, 0.04}, {-0.956, 0.044}, {-0.952, 0.048}, {-0.948, 0.052}, {-0.944, 0.056}, {-0.94, 0.06}, {-0.936, 0.064}, {-0.932, 0.068}, {-0.928, 0.072}, {-0.924, 0.076}, {-0.92, 0.08}, {-0.916, 0.084}, {-0.912, 0.088}, {-0.908, 0.092}, {-0.904, 0.096}, {-0.9, 0.1}, {-0.896, 0.104}, {-0.892, 0.108}, {-0.888, 0.112}, {-0.884, 0.116}, {-0.88, 0.12}, {-0.876, 0.124}, {-0.872, 0.128}, {-0.868, 0.132}, {-0.864, 0.136}, {-0.86, 0.14}, {-0.856, 0.144}, {-0.852, 0.148}, {-0.848, 0.152}, {-0.844, 0.156}, {-0.84, 0.16}, {-0.836, 0.164}, {-0.832, 0.168}, {-0.828, 0.172}, {-0.824, 0.176}, {-0.82, 0.18}, {-0.816, 0.184}, {-0.812, 0.188}, {-0.808, 0.192}, {-0.804, 0.196}, {-0.8, 0.2}, {-0.796, 0.204}, {-0.792, 0.208}, {-0.788, 0.212}, {-0.784, 0.216}, {-0.78, 0.22}, {-0.776, 0.224}, {-0.772, 0.228}, {-0.768, 0.232}, {-0.764, 0.236}, {-0.76, 0.24}, {-0.756, 0.244}, {-0.752, 0.248}, {-0.748, 0.252}, {-0.744, 0.256}, {-0.74, 0.26}, {-0.736, 0.264}, {-0.732, 0.268}, {-0.728, 0.272}, {-0.724, 0.276}, {-0.72, 0.28}, {-0.716, 0.284}, {-0.712, 0.288}, {-0.708, 0.292}, {-0.704, 0.296}, {-0.7, 0.3}, {-0.696, 0.304}, {-0.692, 0.308}, {-0.688, 0.312}, {-0.684, 0.316}, {-0.68, 0.32}, {-0.676, 0.324}, {-0.672, 0.328}, {-0.668, 0.332}, {-0.664, 0.336}, {-0.66, 0.34}, {-0.656, 0.344}, {-0.652, 0.348}, {-0.648, 0.352}, {-0.644, 0.356}, {-0.64, 0.36}, {-0.636, 0.364}, {-0.632, 0.368}, {-0.628, 0.372}, {-0.624, 0.376}, {-0.62, 0.38}, {-0.616, 0.384}, {-0.612, 0.388}, {-0.608, 0.392}, {-0.604, 0.396}, {-0.6, 0.4}, {-0.596, 0.404}, {-0.592, 0.408}, {-0.588, 0.412}, {-0.584, 0.416}, {-0.58, 0.42}, {-0.576, 0.424}, {-0.572, 0.428}, {-0.568, 0.432}, {-0.564, 0.436}, {-0.56, 0.44}, {-0.556, 0.444}, {-0.552, 0.448}, {-0.548, 0.452}, {-0.544, 0.456}, {-0.54, 0.46}, {-0.536, 0.464}, {-0.532, 0.468}, {-0.528, 0.472}, {-0.524, 0.476}, {-0.52, 0.48}, {-0.516, 0.484}, {-0.512, 0.488}, {-0.508, 0.492}, {-0.504, 0.496}, {-0.5, 0.5}, {-0.496, 0.504}, {-0.492, 0.508}, {-0.488, 0.512}, {-0.484, 0.516}, {-0.48, 0.52}, {-0.476, 0.524}, {-0.472, 0.528}, {-0.468, 0.532}, {-0.464, 0.536}, {-0.46, 0.54}, {-0.456, 0.544}, {-0.452, 0.548}, {-0.448, 0.552}, {-0.444, 0.556}, {-0.44, 0.56}, {-0.436, 0.564}, {-0.432, 0.568}, {-0.428, 0.572}, {-0.424, 0.576}, {-0.42, 0.58}, {-0.416, 0.584}, {-0.412, 0.588}, {-0.408, 0.592}, {-0.404, 0.596}, {-0.4, 0.6}, {-0.396, 0.604}, {-0.392, 0.608}, {-0.388, 0.612}, {-0.384, 0.616}, {-0.38, 0.62}, {-0.376, 0.624}, {-0.372, 0.628}, {-0.368, 0.632}, {-0.364, 0.636}, {-0.36, 0.64}, {-0.356, 0.644}, {-0.352, 0.648}, {-0.348, 0.652}, {-0.344, 0.656}, {-0.34, 0.66}, {-0.336, 0.664}, {-0.332, 0.668}, {-0.328, 0.672}, {-0.324, 0.676}, {-0.32, 0.68}, {-0.316, 0.684}, {-0.312, 0.688}, {-0.308, 0.692}, {-0.304, 0.696}, {-0.3, 0.7}, {-0.296, 0.704}, {-0.292, 0.708}, {-0.288, 0.712}, {-0.284, 0.716}, {-0.28, 0.72}, {-0.276, 0.724}, {-0.272, 0.728}, {-0.268, 0.732}, {-0.264, 0.736}, {-0.26, 0.74}, {-0.256, 0.744}, {-0.252, 0.748}, {-0.248, 0.752}, {-0.244, 0.756}, {-0.24, 0.76}, {-0.236, 0.764}, {-0.232, 0.768}, {-0.228, 0.772}, {-0.224, 0.776}, {-0.22, 0.78}, {-0.216, 0.784}, {-0.212, 0.788}, {-0.208, 0.792}, {-0.204, 0.796}, {-0.2, 0.8}, {-0.196, 0.804}, {-0.192, 0.808}, {-0.188, 0.812}, {-0.184, 0.816}, {-0.18, 0.82}, {-0.176, 0.824}, {-0.172, 0.828}, {-0.168, 0.832}, {-0.164, 0.836}, {-0.16, 0.84}, {-0.156, 0.844}, {-0.152, 0.848}, {-0.148, 0.852}, {-0.144, 0.856}, {-0.14, 0.86}, {-0.136, 0.864}, {-0.132, 0.868}, {-0.128, 0.872}, {-0.124, 0.876}, {-0.12, 0.88}, {-0.116, 0.884}, {-0.112, 0.888}, {-0.108, 0.892}, {-0.104, 0.896}, {-0.1, 0.9}, {-0.096, 0.904}, {-0.092, 0.908}, {-0.088, 0.912}, {-0.084, 0.916}, {-0.08, 0.92}, {-0.076, 0.924}, {-0.072, 0.928}, {-0.068, 0.932}, {-0.064, 0.936}, {-0.06, 0.94}, {-0.056, 0.944}, {-0.052, 0.948}, {-0.048, 0.952}, {-0.044, 0.956}, {-0.04, 0.96}, {-0.036, 0.964}, {-0.032, 0.968}, {-0.028, 0.972}, {-0.024, 0.976}, {-0.02, 0.98}, {-0.016, 0.984}, {-0.012, 0.988}, {-0.008, 0.992}, {-0.004, 0.996}, {8.60423e-16, 1.}, {0.004, 1.004}, {0.008, 1.008}, {0.012, 1.012}, {0.016, 1.016}, {0.02, 1.02}, {0.024, 1.024}, {0.028, 1.028}, {0.032, 1.032}, {0.036, 1.036}, {0.04, 1.04}, {0.044, 1.044}, {0.048, 1.048}, {0.052, 1.052}, {0.056, 1.056}, {0.06, 1.06}, {0.064, 1.064}, {0.068, 1.068}, {0.072, 1.072}, {0.076, 1.076}, {0.08, 1.08}, {0.084, 1.084}, {0.088, 1.088}, {0.092, 1.092}, {0.096, 1.096}, {0.1, 1.1}, {0.104, 1.104}, {0.108, 1.108}, {0.112, 1.112}, {0.116, 1.116}, {0.12, 1.12}, {0.124, 1.124}, {0.128, 1.128}, {0.132, 1.132}, {0.136, 1.136}, {0.14, 1.14}, {0.144, 1.144}, {0.148, 1.148}, {0.152, 1.152}, {0.156, 1.156}, {0.16, 1.16}, {0.164, 1.164}, {0.168, 1.168}, {0.172, 1.172}, {0.176, 1.176}, {0.18, 1.18}, {0.184, 1.184}, {0.188, 1.188}, {0.192, 1.192}, {0.196, 1.196}, {0.2, 1.2}, {0.204, 1.204}, {0.208, 1.208}, {0.212, 1.212}, {0.216, 1.216}, {0.22, 1.22}, {0.224, 1.224}, {0.228, 1.228}, {0.232, 1.232}, {0.236, 1.236}, {0.24, 1.24}, {0.244, 1.244}, {0.248, 1.248}, {0.252, 1.252}, {0.256, 1.256}, {0.26, 1.26}, {0.264, 1.264}, {0.268, 1.268}, {0.272, 1.272}, {0.276, 1.276}, {0.28, 1.28}, {0.284, 1.284}, {0.288, 1.288}, {0.292, 1.292}, {0.296, 1.296}, {0.3, 1.3}, {0.304, 1.304}, {0.308, 1.308}, {0.312, 1.312}, {0.316, 1.316}, {0.32, 1.32}, {0.324, 1.324}, {0.328, 1.328}, {0.332, 1.332}, {0.336, 1.336}, {0.34, 1.34}, {0.344, 1.344}, {0.348, 1.348}, {0.352, 1.352}, {0.356, 1.356}, {0.36, 1.36}, {0.364, 1.364}, {0.368, 1.368}, {0.372, 1.372}, {0.376, 1.376}, {0.38, 1.38}, {0.384, 1.384}, {0.388, 1.388}, {0.392, 1.392}, {0.396, 1.396}, {0.4, 1.4}, {0.404, 1.404}, {0.408, 1.408}, {0.412, 1.412}, {0.416, 1.416}, {0.42, 1.42}, {0.424, 1.424}, {0.428, 1.428}, {0.432, 1.432}, {0.436, 1.436}, {0.44, 1.44}, {0.444, 1.444}, {0.448, 1.448}, {0.452, 1.452}, {0.456, 1.456}, {0.46, 1.46}, {0.464, 1.464}, {0.468, 1.468}, {0.472, 1.472}, {0.476, 1.476}, {0.48, 1.48}, {0.484, 1.484}, {0.488, 1.488}, {0.492, 1.492}, {0.496, 1.496}, {0.5, 1.5}, {0.504, 1.504}, {0.508, 1.508}, {0.512, 1.512}, {0.516, 1.516}, {0.52, 1.52}, {0.524, 1.524}, {0.528, 1.528}, {0.532, 1.532}, {0.536, 1.536}, {0.54, 1.54}, {0.544, 1.544}, {0.548, 1.548}, {0.552, 1.552}, {0.556, 1.556}, {0.56, 1.56}, {0.564, 1.564}, {0.568, 1.568}, {0.572, 1.572}, {0.576, 1.576}, {0.58, 1.58}, {0.584, 1.584}, {0.588, 1.588}, {0.592, 1.592}, {0.596, 1.596}, {0.6, 1.6}, {0.604, 1.604}, {0.608, 1.608}, {0.612, 1.612}, {0.616, 1.616}, {0.62, 1.62}, {0.624, 1.624}, {0.628, 1.628}, {0.632, 1.632}, {0.636, 1.636}, {0.64, 1.64}, {0.644, 1.644}, {0.648, 1.648}, {0.652, 1.652}, {0.656, 1.656}, {0.66, 1.66}, {0.664, 1.664}, {0.668, 1.668}, {0.672, 1.672}, {0.676, 1.676}, {0.68, 1.68}, {0.684, 1.684}, {0.688, 1.688}, {0.692, 1.692}, {0.696, 1.696}, {0.7, 1.7}, {0.704, 1.704}, {0.708, 1.708}, {0.712, 1.712}, {0.716, 1.716}, {0.72, 1.72}, {0.724, 1.724}, {0.728, 1.728}, {0.732, 1.732}, {0.736, 1.736}, {0.74, 1.74}, {0.744, 1.744}, {0.748, 1.748}, {0.752, 1.752}, {0.756, 1.756}, {0.76, 1.76}, {0.764, 1.764}, {0.768, 1.768}, {0.772, 1.772}, {0.776, 1.776}, {0.78, 1.78}, {0.784, 1.784}, {0.788, 1.788}, {0.792, 1.792}, {0.796, 1.796}, {0.8, 1.8}, {0.804, 1.804}, {0.808, 1.808}, {0.812, 1.812}, {0.816, 1.816}, {0.82, 1.82}, {0.824, 1.824}, {0.828, 1.828}, {0.832, 1.832}, {0.836, 1.836}, {0.84, 1.84}, {0.844, 1.844}, {0.848, 1.848}, {0.852, 1.852}, {0.856, 1.856}, {0.86, 1.86}, {0.864, 1.864}, {0.868, 1.868}, {0.872, 1.872}, {0.876, 1.876}, {0.88, 1.88}, {0.884, 1.884}, {0.888, 1.888}, {0.892, 1.892}, {0.896, 1.896}, {0.9, 1.9}, {0.904, 1.904}, {0.908, 1.908}, {0.912, 1.912}, {0.916, 1.916}, {0.92, 1.92}, {0.924, 1.924}, {0.928, 1.928}, {0.932, 1.932}, {0.936, 1.936}, {0.94, 1.94}, {0.944, 1.944}, {0.948, 1.948}, {0.952, 1.952}, {0.956, 1.956}, {0.96, 1.96}, {0.964, 1.964}, {0.968, 1.968}, {0.972, 1.972}, {0.976, 1.976}, {0.98, 1.98}, {0.984, 1.984}, {0.988, 1.988}, {0.992, 1.992}, {0.996, 1.996}}]}}, {DisplayFunction -> Identity, AspectRatio -> (1/(GoldenRatio)), Axes -> {True, True}, AxesLabel -> {None, None}, AxesOrigin -> {0, 0}, DisplayFunction :> Identity, Frame -> {{False, False}, {False, False}}, FrameLabel -> {{None, None}, {None, None}}, FrameTicks -> {{Automatic, Automatic}, {Automatic, Automatic}}, GridLines -> {None, None}, GridLinesStyle -> Directive[GrayLevel[0.5, 0.4]], Method -> {"DefaultBoundaryStyle" -> Automatic, "DefaultMeshStyle" -> AbsolutePointSize[6], "ScalingFunctions" -> None}, PlotRange -> {{-1, 1}, {0., 1.996}}, PlotRangeClipping -> True, PlotRangePadding -> {{Scaled[0.02], Scaled[0.02]}, {Scaled[0.05], Scaled[0.05]}}, Ticks -> {Automatic, Automatic}}]
`

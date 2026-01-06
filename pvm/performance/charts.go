package performance

import (
	"fmt"
	"image/color"
	"os"
	"path/filepath"
	"sort"
	"time"

	"gonum.org/v1/plot"
	"gonum.org/v1/plot/plotter"
	"gonum.org/v1/plot/vg"
)

// ChartConfig holds configuration for chart generation
type ChartConfig struct {
	OutputDir string
	Width     vg.Length
	Height    vg.Length
}

// DefaultChartConfig returns default chart configuration
func DefaultChartConfig() *ChartConfig {
	return &ChartConfig{
		OutputDir: "results",
		Width:     12 * vg.Inch,
		Height:    6 * vg.Inch,
	}
}

// GenerateAllCharts generates compile speed chart
func GenerateAllCharts(stats []*CompileStats, config *ChartConfig) error {
	if config == nil {
		config = DefaultChartConfig()
	}

	// Create output directory
	if err := os.MkdirAll(config.OutputDir, 0755); err != nil {
		return fmt.Errorf("failed to create output directory: %w", err)
	}

	// Generate compile speed chart (overwrites existing file)
	filename := filepath.Join(config.OutputDir, "compile_speed.png")
	if err := generateCompileSpeedChart(stats, config, filename); err != nil {
		return fmt.Errorf("failed to generate compile_speed chart: %w", err)
	}
	fmt.Printf("Generated: %s\n", filename)

	return nil
}

// generateCompileSpeedChart creates a scatter plot of compile time vs instruction count
func generateCompileSpeedChart(stats []*CompileStats, config *ChartConfig, filename string) error {
	p := plot.New()
	p.Title.Text = "Compile Speed: Time vs PVM Instructions"
	p.X.Label.Text = "PVM Instruction Count"
	p.Y.Label.Text = "Compile Time (ms)"

	pts := make(plotter.XYs, len(stats))
	for i, s := range stats {
		pts[i].X = float64(s.PVMInstructionCount)
		pts[i].Y = float64(s.CompileTime.Milliseconds())
	}

	scatter, err := plotter.NewScatter(pts)
	if err != nil {
		return err
	}
	scatter.GlyphStyle.Color = color.RGBA{R: 66, G: 133, B: 244, A: 255}
	scatter.GlyphStyle.Radius = vg.Points(5)

	p.Add(scatter)
	p.Add(plotter.NewGrid())

	return p.Save(config.Width, config.Height, filename)
}

var backendColorMap = map[string]color.RGBA{
	"interpreter": {R: 66, G: 133, B: 244, A: 255},
	"compiler":    {R: 219, G: 68, B: 55, A: 255},
	"sandbox":     {R: 244, G: 180, B: 0, A: 255},
}

var backendFallbackColors = []color.RGBA{
	{R: 15, G: 157, B: 88, A: 255},
	{R: 171, G: 71, B: 188, A: 255},
	{R: 0, G: 172, B: 193, A: 255},
	{R: 102, G: 102, B: 102, A: 255},
}

func orderedBackends(series map[string]plotter.XYs) []string {
	var order []string
	if _, ok := series["interpreter"]; ok {
		order = append(order, "interpreter")
	}
	if _, ok := series["compiler"]; ok {
		order = append(order, "compiler")
	}
	if _, ok := series["sandbox"]; ok {
		order = append(order, "sandbox")
	}

	var rest []string
	for backend := range series {
		if backend == "interpreter" || backend == "compiler" || backend == "sandbox" {
			continue
		}
		rest = append(rest, backend)
	}
	sort.Strings(rest)
	order = append(order, rest...)
	return order
}

func backendColor(backend string, fallbackIndex int) color.Color {
	if c, ok := backendColorMap[backend]; ok {
		return c
	}
	if len(backendFallbackColors) == 0 {
		return color.RGBA{R: 102, G: 102, B: 102, A: 255}
	}
	return backendFallbackColors[fallbackIndex%len(backendFallbackColors)]
}

// GenerateBackendRuntimeChart creates a line chart for backend average runtime vs iterations.
func GenerateBackendRuntimeChart(results []BackendBenchResult, config *ChartConfig, filename string) error {
	if config == nil {
		config = DefaultChartConfig()
	}

	series := make(map[string]plotter.XYs)
	for _, r := range results {
		y := float64(r.AvgNs) / float64(time.Millisecond)
		series[r.Backend] = append(series[r.Backend], plotter.XY{
			X: float64(r.Iterations),
			Y: y,
		})
	}

	p := plot.New()
	p.Title.Text = "Backend Runtime: Interpreter vs Recompiler"
	p.X.Label.Text = "Iterations"
	p.Y.Label.Text = "Average Duration (ms)"

	order := orderedBackends(series)
	for i, backend := range order {
		pts := series[backend]
		sort.Slice(pts, func(i, j int) bool { return pts[i].X < pts[j].X })

		line, points, err := plotter.NewLinePoints(pts)
		if err != nil {
			return err
		}
		color := backendColor(backend, i)
		line.Color = color
		line.Width = vg.Points(2)
		points.Color = color
		points.Radius = vg.Points(4)

		p.Add(line, points)
		p.Legend.Add(backend, line, points)
	}

	p.Add(plotter.NewGrid())
	return p.Save(config.Width, config.Height, filename)
}

// GenerateBackendSpeedupChart creates a speedup plot for baseBackend/compareBackend.
func GenerateBackendSpeedupChart(results []BackendBenchResult, config *ChartConfig, filename, baseBackend, compareBackend string) error {
	if config == nil {
		config = DefaultChartConfig()
	}

	baseByIter := make(map[uint64]float64)
	compareByIter := make(map[uint64]float64)
	for _, r := range results {
		if r.Backend == baseBackend {
			baseByIter[r.Iterations] = float64(r.AvgNs)
		}
		if r.Backend == compareBackend {
			compareByIter[r.Iterations] = float64(r.AvgNs)
		}
	}

	var pts plotter.XYs
	for iter, baseNs := range baseByIter {
		compareNs, ok := compareByIter[iter]
		if !ok || compareNs == 0 {
			continue
		}
		pts = append(pts, plotter.XY{
			X: float64(iter),
			Y: baseNs / compareNs,
		})
	}
	sort.Slice(pts, func(i, j int) bool { return pts[i].X < pts[j].X })

	p := plot.New()
	p.Title.Text = fmt.Sprintf("Speedup: %s / %s", baseBackend, compareBackend)
	p.X.Label.Text = "Iterations"
	p.Y.Label.Text = "Speedup (x)"

	line, points, err := plotter.NewLinePoints(pts)
	if err != nil {
		return err
	}
	color := backendColor(baseBackend, 0)
	line.Color = color
	line.Width = vg.Points(2)
	points.Color = color
	points.Radius = vg.Points(4)

	p.Add(line, points)
	p.Add(plotter.NewGrid())
	return p.Save(config.Width, config.Height, filename)
}

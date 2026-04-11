package main

import (
	"flag"
	"fmt"
	"image/color"
	"log"
	"math"
	"math/rand"
	"os"
	"path/filepath"
	"time"

	"gonum.org/v1/plot"
	"gonum.org/v1/plot/plotter"
	"gonum.org/v1/plot/vg"
)

// 模型参数
type Params struct {
	S0          float64 // 初始价格
	Sigma       float64 // 波动率（年化）
	TimeYears   float64 // 总时长（年）
	DtSeconds   float64 // 每步间隔（秒）
	Steps       int     // 模拟步数
}

// 生成算术随机游走路径
func arithmeticRandomWalk(p Params, rng *rand.Rand) plotter.XYs {
	dt := p.TimeYears / float64(p.Steps)
	sigmaDt := p.Sigma * math.Sqrt(dt) // 每步绝对波动幅度
	path := make(plotter.XYs, p.Steps+1)
	path[0].X = 0
	path[0].Y = p.S0

	S := p.S0
	for i := 1; i <= p.Steps; i++ {
		// 50% 向上 +sigmaDt，50% 向下 -sigmaDt
		if rng.Float64() < 0.5 {
			S += sigmaDt
		} else {
			S -= sigmaDt
		}
		path[i].X = float64(i) * p.DtSeconds
		path[i].Y = S
	}
	return path
}

// 生成几何布朗运动路径（连续复利）
func geometricBrownianMotion(p Params, rng *rand.Rand) plotter.XYs {
	dt := p.TimeYears / float64(p.Steps)
	// 对数收益率的漂移和扩散
	mu := 0.0 // 假设无风险利率为0，仅关注波动
	drift := (mu - 0.5*p.Sigma*p.Sigma) * dt
	vol := p.Sigma * math.Sqrt(dt)

	path := make(plotter.XYs, p.Steps+1)
	path[0].X = 0
	path[0].Y = p.S0

	S := p.S0
	for i := 1; i <= p.Steps; i++ {
		z := rng.NormFloat64()
		logReturn := drift + vol*z
		S *= math.Exp(logReturn)
		path[i].X = float64(i) * p.DtSeconds
		path[i].Y = S
	}
	return path
}

func main() {
	// 命令行参数
	S0 := flag.Float64("s0", 1000, "初始价格")
	sigma := flag.Float64("sigma", 3, "年化波动率 (例如 0.2 表示20%)")
	duration := flag.Duration("duration", 5*time.Minute, "模拟时长 (例如 30s, 10m, 24h)")
	interval := flag.Duration("interval", 100*time.Millisecond, "每步间隔时间 (例如 10ms, 1s)")
	seed := flag.Int64("seed", time.Now().UnixNano(), "随机种子")
	output := flag.String("o", ".temp/rand-k-line.png", "输出文件路径")
	flag.Parse()

	// 计算总步数和时间（以年为单位）
	timeYears := duration.Seconds() / (365.25 * 24 * 3600)
	totalSteps := int(duration.Seconds() / interval.Seconds())

	params := Params{
		S0:        *S0,
		Sigma:     *sigma,
		TimeYears: timeYears,
		DtSeconds: interval.Seconds(),
		Steps:     totalSteps,
	}

	fmt.Printf("模拟参数: 初始价=%.2f, 波动率=%.2f%%, 时长=%s, 间隔=%s, 步数=%d\n",
		params.S0, params.Sigma*100, *duration, *interval, params.Steps)

	// 创建随机数生成器
	rng := rand.New(rand.NewSource(*seed))

	// 生成两条路径
	pathArith := arithmeticRandomWalk(params, rng)
	pathGBM := geometricBrownianMotion(params, rng)

	// 创建绘图
	p := plot.New()
	p.Title.Text = "Stochastic Model Comparison"
	p.X.Label.Text = "Time (s)"
	p.Y.Label.Text = "Price"

	// 添加两条线
	addLine := func(data plotter.XYs, name string, col color.Color) {
		line, err := plotter.NewLine(data)
		if err != nil {
			log.Fatal(err)
		}
		line.Color = col
		p.Add(line)
		p.Legend.Add(name, line)
	}

	addLine(pathArith, "Arithmetic Random Walk", color.RGBA{R: 255, G: 0, B: 0, A: 255})  // 红色
	addLine(pathGBM, "Geometric Brownian Motion", color.RGBA{R: 0, G: 0, B: 255, A: 255}) // 蓝色

	p.Legend.Top = true
	p.Legend.Left = true

	// 保存图片
	if err := os.MkdirAll(filepath.Dir(*output), 0o755); err != nil {
		log.Fatal(err)
	}
	if err := p.Save(8*vg.Inch, 6*vg.Inch, *output); err != nil {
		log.Fatal(err)
	}
	fmt.Printf("图表已保存为 %s\n", *output)
}

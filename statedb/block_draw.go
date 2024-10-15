package statedb

import (
	"fmt"
	"log"
	"net/http"

	"github.com/go-echarts/go-echarts/v2/charts"
	"github.com/go-echarts/go-echarts/v2/components"
	"github.com/go-echarts/go-echarts/v2/opts"
	"github.com/go-echarts/go-echarts/v2/types"
)

type Block_draw struct {
	Hash      string
	Parent    string
	AuthorIdx int
	Timeslot  string
}

var (
	drawblocks = make(map[string]*Block_draw)
	edges      []opts.GraphLink
	graph      *charts.Graph
)

func AddDrawBlock(blockHash, parentHash string, authorIdx int, timeslot string) {
	if _, exists := drawblocks[blockHash]; !exists {
		drawblocks[blockHash] = &Block_draw{
			Hash:      blockHash,
			Parent:    parentHash,
			AuthorIdx: authorIdx,
			Timeslot:  timeslot,
		}
		edges = append(edges, opts.GraphLink{Source: parentHash, Target: blockHash})
	}

	if parentHash != "" && !existsInDrawBlocks(parentHash) {
		drawblocks[parentHash] = &Block_draw{Hash: parentHash}
	}
}

func existsInDrawBlocks(hash string) bool {
	_, exists := drawblocks[hash]
	return exists
}

func setupGraph() *charts.Graph {
	graph := charts.NewGraph()
	graph.SetGlobalOptions(
		charts.WithTitleOpts(opts.Title{
			Title:    "Blockchain Fork Visualization",
			Subtitle: "Dynamic representation of block forks",
		}),
		charts.WithTooltipOpts(opts.Tooltip{Show: opts.Bool(true)}),
	)

	nodes, links := getGraphData()
	graph.AddSeries("Blockchain", nodes, links).SetSeriesOptions(
		charts.WithGraphChartOpts(opts.GraphChart{
			Force:  &opts.GraphForce{Repulsion: 1000, Gravity: 0.3},
			Layout: "force",
			Roam:   opts.Bool(true),
		}),
		charts.WithLabelOpts(opts.Label{Show: opts.Bool(true), Position: "right", Formatter: "{b}"}),
	)
	return graph
}

func getGraphData() ([]opts.GraphNode, []opts.GraphLink) {
	nodes := make([]opts.GraphNode, 0, len(drawblocks))
	links := make([]opts.GraphLink, 0, len(edges))

	for _, block := range drawblocks {
		nodes = append(nodes, opts.GraphNode{
			Name: block.Hash,
			Tooltip: &opts.Tooltip{
				Show:      opts.Bool(true),
				Formatter: types.FuncStr(fmt.Sprintf("Block: %s,Author: %d,Timeslot: %s", block.Hash, block.AuthorIdx, block.Timeslot)),
			},
		})
	}
	for _, edge := range edges {
		links = append(links, opts.GraphLink{
			Source: edge.Source,
			Target: edge.Target,
		})
	}
	return nodes, links
}

func RunGraph() {
	http.HandleFunc("/", func(rw http.ResponseWriter, req *http.Request) {
		graph := setupGraph()
		page := components.NewPage()
		page.AddCharts(graph)
		page.Render(rw)
	})

	log.Println("Starting server at :3030")
	if err := http.ListenAndServe(":3030", nil); err != nil {
		log.Fatalf("Failed to start server: %v", err)
	}
}

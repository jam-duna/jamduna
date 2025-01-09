package types

import (
	"fmt"
	"log"
	"net/http"

	"github.com/go-echarts/go-echarts/v2/charts"
	"github.com/go-echarts/go-echarts/v2/components"
	"github.com/go-echarts/go-echarts/v2/opts"
	"github.com/go-echarts/go-echarts/v2/types"
)

type GraphServer struct {
	Graph  *charts.Graph
	Server *http.Server
}

// NewGraphServer initializes a new GraphServer instance.
func NewGraphServer() *GraphServer {
	return &GraphServer{
		Graph: charts.NewGraph(),
	}
}

// StartServer sets up an HTTP server to serve the visualization.
func (gs *GraphServer) StartServer() {
	http.HandleFunc("/", func(rw http.ResponseWriter, req *http.Request) {
		page := components.NewPage()
		if gs.Graph != nil {
			page.AddCharts(gs.Graph)
		}
		page.Render(rw)
	})

	log.Println("Starting server at :3030")
	if err := http.ListenAndServe(":3030", nil); err != nil && err != http.ErrServerClosed {
		log.Fatalf("Failed to start server: %v", err)
	}
}

func (gs *GraphServer) Update(bt *BlockTree) {
	if bt == nil {
		return
	}
	bt.Mutex.RLock()
	defer bt.Mutex.RUnlock()

	gs.Graph = charts.NewGraph()
	gs.Graph.SetGlobalOptions(
		charts.WithTitleOpts(opts.Title{
			Title:    "BlockTree Visualization",
			Subtitle: "Dynamic representation of the block tree",
		}),
		charts.WithTooltipOpts(opts.Tooltip{Show: opts.Bool(true)}),
	)

	nodes, links := getGraphData(bt)
	gs.Graph.AddSeries("BlockTree", nodes, links).SetSeriesOptions(
		charts.WithGraphChartOpts(opts.GraphChart{
			Force:  &opts.GraphForce{Repulsion: 1000, Gravity: 0.3},
			Layout: "force",
			Roam:   opts.Bool(true),
		}),
		charts.WithLabelOpts(opts.Label{Show: opts.Bool(true), Position: "right", Formatter: "{b}"}),
	)
}

// getGraphData generates the nodes and links for the graph.
func getGraphData(bt *BlockTree) ([]opts.GraphNode, []opts.GraphLink) {
	nodes := make([]opts.GraphNode, 0, len(bt.TreeMap))
	links := make([]opts.GraphLink, 0)

	for _, node := range bt.TreeMap {
		// Add each node to the graph.
		color := "red"
		if node.Finalized {
			color = "green"
		}

		nodes = append(nodes, opts.GraphNode{
			Name:  node.Block.Hash().String_short(),
			Value: float32(node.Height),
			Tooltip: &opts.Tooltip{
				Show:      opts.Bool(true),
				Formatter: types.FuncStr(fmt.Sprintf("Block: %s<br>Height: %d<br>Finalized: %v, weights %d", node.Block.Hash(), node.Height, node.Finalized, node.Cumulative_VotseWeight)),
			},
			ItemStyle: &opts.ItemStyle{
				Color: color,
			},
		})

		// Add links from parent to child nodes.
		if node.Parent != nil {
			links = append(links, opts.GraphLink{
				Source: node.Parent.Block.Hash().String_short(),
				Target: node.Block.Hash().String_short(),
			})
		}
	}

	return nodes, links
}

package main

import (
	"fmt"
	"image"
	"image/color"
	"log"
	"math/rand"
	"os"
	"runtime/pprof"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/inpututil"
	"github.com/hajimehoshi/ebiten/v2/vector"
)

var (
	blackImage    = ebiten.NewImage(3, 3)
	blackSubImage = blackImage.SubImage(image.Rect(1, 1, 2, 2)).(*ebiten.Image)
)

type Images struct {
	Black, White *ebiten.Image
}

type Grid struct {
	Data                   [][]int64
	Width, Height, Density int
}

// Create new empty grid and allocate Data according to provided dimensions
func NewGrid(width, height, density int) *Grid {
	grid := &Grid{
		Height:  height,
		Width:   width,
		Density: density,
		Data:    make([][]int64, height),
	}

	for y := 0; y < height; y++ {
		grid.Data[y] = make([]int64, width)
	}

	return grid
}

// live console output of the grid
func (grid *Grid) Dump() {
	/*
		cmd := exec.Command("clear")
		cmd.Stdout = os.Stdout
		cmd.Run()

		for y := 0; y < grid.Height; y++ {
			for x := 0; x < grid.Width; x++ {
				if grid.Data[y][x] == 1 {
					fmt.Print("XX")
				} else {
					fmt.Print("  ")
				}
			}
			fmt.Println()
		}
	*/
	fmt.Printf("FPS: %0.2f\n", ebiten.ActualTPS())
}

type Game struct {
	Width, Height, Cellsize, Density int
	ScreenWidth, ScreenHeight        int
	Grids                            []*Grid
	Index                            int
	Black, White, Grey               color.RGBA
	Tiles                            Images
	Cache                            *ebiten.Image
	Elapsed                          int64
	TPG                              int64 // adjust game speed independently of TPS
	Vertices                         []ebiten.Vertex
	Indices                          []uint16
	Pause, Debug                     bool
}

// fill a cell
func FillCell(tile *ebiten.Image, cellsize int, col color.RGBA) {
	vector.DrawFilledRect(
		tile,
		float32(1),
		float32(1),
		float32(cellsize-1),
		float32(cellsize-1),
		col, false,
	)
}

func (game *Game) Layout(outsideWidth, outsideHeight int) (int, int) {
	return game.ScreenWidth, game.ScreenHeight
}

func (game *Game) Init() {
	// setup two grids, one for display, one for next state
	grida := NewGrid(game.Width, game.Height, game.Density)
	gridb := NewGrid(game.Width, game.Height, game.Density)

	for y := 0; y < game.Height; y++ {
		for x := 0; x < game.Width; x++ {
			if rand.Intn(game.Density) == 1 {
				grida.Data[y][x] = 1
			}
		}

	}

	game.Grids = []*Grid{
		grida,
		gridb,
	}

	// setup colors
	game.Grey = color.RGBA{128, 128, 128, 0xff}
	game.Black = color.RGBA{0, 0, 0, 0xff}
	game.White = color.RGBA{200, 200, 200, 0xff}

	game.Tiles.White = ebiten.NewImage(game.Cellsize, game.Cellsize)
	game.Cache = ebiten.NewImage(game.ScreenWidth, game.ScreenHeight)

	FillCell(game.Tiles.White, game.Cellsize, game.White)
	game.Cache.Fill(game.Grey)

	// draw the offscreen image
	op := &ebiten.DrawImageOptions{}
	for y := 0; y < game.Height; y++ {
		for x := 0; x < game.Width; x++ {
			op.GeoM.Reset()
			op.GeoM.Translate(float64(x*game.Cellsize), float64(y*game.Cellsize))
			game.Cache.DrawImage(game.Tiles.White, op)
		}
	}

	blackSubImage.Fill(game.Black)

	lenvertices := game.ScreenHeight * game.ScreenWidth
	game.Vertices = make([]ebiten.Vertex, lenvertices)
	game.Indices = make([]uint16, lenvertices+(lenvertices/2))
}

// count the living neighbors of a cell
func (game *Game) CountNeighbors(x, y int) int64 {
	var sum int64

	for nbgX := -1; nbgX < 2; nbgX++ {
		for nbgY := -1; nbgY < 2; nbgY++ {
			var col, row int

			// Wrap  mode we look  at all the 8  neighbors surrounding
			//  us.  In  case we  are  on an  edge we'll  look at  the
			// neighbor on  the other side of the  grid, thus wrapping
			// lookahead around using the mod() function.
			col = (x + nbgX + game.Width) % game.Width
			row = (y + nbgY + game.Height) % game.Height

			sum += game.Grids[game.Index].Data[row][col]
		}
	}

	// don't count ourselfes though
	sum -= game.Grids[game.Index].Data[y][x]

	return sum
}

// the heart of the game
func (game *Game) CheckRule(state int64, neighbors int64) int64 {
	var nextstate int64

	if state == 0 && neighbors == 3 {
		nextstate = 1
	} else if state == 1 && (neighbors == 2 || neighbors == 3) {
		nextstate = 1
	} else {
		nextstate = 0
	}

	return nextstate
}

// we only  update the cells if  we are not  in pause state or  if the
// game timer (TPG) is elapsed.
func (game *Game) UpdateCells() {
	if game.Pause {
		return
	}

	if game.Elapsed < game.TPG {
		game.Elapsed++
		return
	}

	// next grid index. we only have to, so we just xor it
	next := game.Index ^ 1

	// reset vertices
	// FIXME: fails!
	game.ClearVertices()

	// calculate cell life state, this is the actual game of life
	for y := 0; y < game.Height; y++ {
		for x := 0; x < game.Width; x++ {
			state := game.Grids[game.Index].Data[y][x] // 0|1 == dead or alive
			neighbors := game.CountNeighbors(x, y)     // alive neighbor count

			// actually apply the current rules
			nextstate := game.CheckRule(state, neighbors)

			// change state of current cell in next grid
			game.Grids[next].Data[y][x] = nextstate
		}
	}

	// calculate triangles for rendering
	game.UpdateTriangles()

	// switch grid for rendering
	game.Index ^= 1

	game.Elapsed = 0

	if game.Debug {
		game.Grids[next].Dump()
	}
}

func (game *Game) Update() error {
	game.UpdateCells()

	if inpututil.IsKeyJustPressed(ebiten.KeySpace) {
		game.Pause = !game.Pause
	}

	return nil
}

func (game *Game) ClearVertices() {
	// FIXME: fails
	for i := 0; i < len(game.Vertices); i++ {
		game.Vertices[i] = ebiten.Vertex{}
		// game.Vertices[i].DstX = 0
		// game.Vertices[i].DstY = 1
	}

	game.Indices = game.Indices[:len(game.Indices)]
}

// create the triangles needed for rendering. Actual rendering doesn't
// happen here but in Draw()
func (game *Game) UpdateTriangles() {
	var base uint16 = 0
	var index uint16 = 0

	idx := 0

	// iterate over every cell
	for celly := 0; celly < game.Height; celly++ {
		for cellx := 0; cellx < game.Width; cellx++ {

			// if the cell is alife
			if game.Grids[game.Index].Data[celly][cellx] == 1 {

				/* iterate over the cell's corners:
				0   1

				2   3
				*/
				for i := 0; i < 2; i++ {
					for j := 0; j < 2; j++ {

						// calculate the corner position
						x := (cellx * game.Cellsize) + (i * game.Cellsize) + 1
						y := (celly * game.Cellsize) + (j * game.Cellsize) + 1

						if i == 1 {
							x -= 1
						}
						if j == 1 {
							y -= 1
						}

						// setup the vertex
						game.Vertices[idx].DstX = float32(x)
						game.Vertices[idx].DstY = float32(y)
						game.Vertices[idx].SrcX = 1
						game.Vertices[idx].SrcY = 1
						game.Vertices[idx].ColorR = float32(game.Black.R)
						game.Vertices[idx].ColorG = float32(game.Black.G)
						game.Vertices[idx].ColorB = float32(game.Black.B)
						game.Vertices[idx].ColorA = 1

						idx++
					}
				}
			}

			// indices for first triangle
			game.Indices[index] = base
			game.Indices[index+1] = base + 1
			game.Indices[index+2] = base + 3

			// for the second one
			game.Indices[index+3] = base
			game.Indices[index+4] = base + 2
			game.Indices[index+5] = base + 3

			index += 6 // 3 indicies per triangle

			base += 4 // 4 vertices per cell
		}
	}
}

func (game *Game) Draw(screen *ebiten.Image) {
	op := &ebiten.DrawImageOptions{}

	op.GeoM.Translate(0, 0)
	screen.DrawImage(game.Cache, op)

	triop := &ebiten.DrawTrianglesOptions{}
	screen.DrawTriangles(game.Vertices, game.Indices, blackSubImage, triop)
}

func main() {
	size := 200

	game := &Game{
		Width:    size,
		Height:   size,
		Cellsize: 4,
		Density:  5,
		TPG:      5,
		Debug:    true,
	}

	game.ScreenWidth = game.Width * game.Cellsize
	game.ScreenHeight = game.Height * game.Cellsize

	game.Init()

	ebiten.SetWindowSize(game.ScreenWidth, game.ScreenHeight)
	ebiten.SetWindowTitle("triangle conway's game of life")
	ebiten.SetWindowResizingMode(ebiten.WindowResizingModeEnabled)

	fd, err := os.Create("cpu.profile")
	if err != nil {
		log.Fatal(err)
	}
	defer fd.Close()

	pprof.StartCPUProfile(fd)
	defer pprof.StopCPUProfile()

	if err := ebiten.RunGame(game); err != nil {
		log.Fatal(err)
	}
}

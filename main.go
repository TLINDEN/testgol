package main

import (
	"fmt"
	"log"
	"math/rand"
	"os"
	"os/exec"
	"runtime/pprof"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/inpututil"
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

type Game struct {
	Width, Height, Cellsize, Density int
	ScreenWidth, ScreenHeight        int
	Grids                            []*Grid
	Index                            int
	Elapsed                          int64
	TPG                              int64 // adjust game speed independently of TPS
	Pause, Debug, Profile, Gridlines bool
	Pixels                           []byte
	OffScreen                        *ebiten.Image
}

func (game *Game) Layout(outsideWidth, outsideHeight int) (int, int) {
	return game.ScreenWidth, game.ScreenHeight
}

// live console output of the grid
func (game *Game) DebugDump() {
	cmd := exec.Command("clear")
	cmd.Stdout = os.Stdout
	cmd.Run()

	if game.Debug {
		for y := 0; y < game.Height; y++ {
			for x := 0; x < game.Width; x++ {
				if game.Grids[game.Index].Data[y][x] == 1 {
					fmt.Print("XX")
				} else {
					fmt.Print("  ")
				}
			}
			fmt.Println()
		}
	}
	fmt.Printf("FPS: %0.2f\n", ebiten.ActualTPS())
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

	game.Pixels = make([]byte, game.ScreenWidth*game.ScreenHeight*4)

	game.OffScreen = ebiten.NewImage(game.ScreenWidth, game.ScreenHeight)
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

	// switch grid for rendering
	game.Index ^= 1

	game.Elapsed = 0

	game.UpdatePixels()
}

func (game *Game) Update() error {
	game.UpdateCells()

	if inpututil.IsKeyJustPressed(ebiten.KeySpace) {
		game.Pause = !game.Pause
	}

	return nil
}

/*
*
r, g, b := color(it)

	78             p := 4 * (i + j*screenWidth)
	79             gm.offscreenPix[p] = r
	80             gm.offscreenPix[p+1] = g
	81             gm.offscreenPix[p+2] = b
	82             gm.offscreenPix[p+3] = 0xff
*/
func (game *Game) UpdatePixels() {
	var col byte

	gridx := 0
	gridy := 0
	idx := 0

	for y := 0; y < game.ScreenHeight; y++ {
		for x := 0; x < game.ScreenWidth; x++ {
			gridx = x / game.Cellsize
			gridy = y / game.Cellsize

			col = 0xff
			if game.Grids[game.Index].Data[gridy][gridx] == 1 {
				col = 0x0
			}

			/*
				if math.Mod(float64(x), float64(game.Cellsize)) == 0 ||
					math.Mod(float64(y), float64(game.Cellsize)) == 0 {
					col = 128
				}
			*/
			if game.Gridlines {
				if x%game.Cellsize == 0 || y%game.Cellsize == 0 {
					col = 128
				}
			}

			idx = 4 * (x + y*game.ScreenWidth)

			game.Pixels[idx] = col
			game.Pixels[idx+1] = col
			game.Pixels[idx+2] = col
			game.Pixels[idx+3] = 0xff

			idx++
		}
	}

	game.OffScreen.WritePixels(game.Pixels)
}

func (game *Game) Draw(screen *ebiten.Image) {
	screen.DrawImage(game.OffScreen, nil)
	game.DebugDump()
}

func main() {
	size := 500

	game := &Game{
		Width:     size,
		Height:    size,
		Cellsize:  4,
		Density:   8,
		TPG:       10,
		Debug:     false,
		Profile:   false,
		GridLines: false,
	}

	game.ScreenWidth = game.Width * game.Cellsize
	game.ScreenHeight = game.Height * game.Cellsize

	game.Init()

	ebiten.SetWindowSize(game.ScreenWidth, game.ScreenHeight)
	ebiten.SetWindowTitle("triangle conway's game of life")
	ebiten.SetWindowResizingMode(ebiten.WindowResizingModeEnabled)

	if game.Profile {
		fd, err := os.Create("cpu.profile")
		if err != nil {
			log.Fatal(err)
		}
		defer fd.Close()

		pprof.StartCPUProfile(fd)
		defer pprof.StopCPUProfile()
	}

	if err := ebiten.RunGame(game); err != nil {
		log.Fatal(err)
	}
}

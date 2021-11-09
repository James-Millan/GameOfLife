package main

import (
	"fmt"
)

func main() {
	fmt.Println("hello")
}

func processSlice() {
	for i := range currentWorld {
		for j := range currentWorld[i] {
			if getNumSurroundingCells(i, j, currentWorld) == 3 {
				nextWorld[i][j] = 0xFF
			} else if getNumSurroundingCells(i, j, currentWorld) < 2 {
				nextWorld[i][j] = 0
			} else if getNumSurroundingCells(i, j, currentWorld) > 3 {
				nextWorld[i][j] = 0
			} else if getNumSurroundingCells(i, j, currentWorld) == 2 {
				nextWorld[i][j] = currentWorld[i][j]
			}
		}
	}
}

//count number of active cells surrounding a current cell
func getNumSurroundingCells(x int, y int, world [][]byte) int {
	const ALIVE = 0xFF
	var counter = 0
	var succX = x + 1
	var succY = y + 1
	var prevX = x - 1
	var prevY = y - 1
	succX = boundNumber(succX, len(world))
	succY = boundNumber(succY, len(world))
	prevX = boundNumber(prevX, len(world))
	prevY = boundNumber(prevY, len(world))
	if world[prevX][y] == ALIVE {
		counter++
	}
	if world[prevX][prevY] == ALIVE {
		counter++
	}
	if world[prevX][succY] == ALIVE {
		counter++
	}
	if world[x][succY] == ALIVE {
		counter++
	}
	if world[x][prevY] == ALIVE {
		counter++
	}
	if world[succX][y] == ALIVE {
		counter++
	}
	if world[succX][succY] == ALIVE {
		counter++
	}
	if world[succX][prevY] == ALIVE {
		counter++
	}
	return counter
}

func boundNumber(num int, worldLen int) int {
	if num < 0 {
		return num + worldLen
	} else if num > worldLen-1 {
		return num - worldLen
	} else {
		return num
	}
}

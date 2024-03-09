package main

import (
	"context"
	"fmt"
	"github.com/stark-sim/cephalon-ent/pkg/cep_ent/missionorder"
	"vpay/internal/db"
)

func main() {
	first, err := db.DB.MissionOrder.Query().Where(missionorder.ID(1727295537231040512)).First(context.Background())
	if err != nil {
		return
	}
	fmt.Println(first)
}

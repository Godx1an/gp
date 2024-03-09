package utils

import (
	"fmt"
	"github.com/bwmarrin/snowflake"
	"github.com/sirupsen/logrus"
	"math/rand"
)

var (
	node *snowflake.Node
	err  error
)

func init() {
	// Create a new Node with a random Node number
	node, err = snowflake.NewNode(rand.Int63n(1024))
	if err != nil {
		logrus.Errorf("failed at creating new snowflake node, err: %v", err)
		panic(fmt.Sprintf("snowflake init fail, %v", err))
	}
	logrus.Infoln("snowflake node init success...")
}

func GenSnowflakeID() int64 {
	if node == nil {
		// 预防横向伸缩时多个 pod 同时运行，最稳妥的是有一个总的分发器
		node, err = snowflake.NewNode(rand.Int63n(1024))
	}
	// Generate a snowflake ID.
	id := node.Generate()
	return id.Int64()
}

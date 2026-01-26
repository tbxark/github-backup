package main

import (
	"flag"
	"fmt"
	"log"

	"github.com/robfig/cron/v3"
	"github.com/tbxark/github-backup/config"
)

var (
	BuildVersion = "dev"
)

func main() {
	conf := flag.String("config", "config.json", "config file")
	version := flag.Bool("version", false, "show version")
	help := flag.Bool("help", false, "show help")
	flag.Parse()
	if *version {
		fmt.Println(BuildVersion)
		return
	}
	if *help {
		flag.Usage()
		return
	}
	data, err := config.NewConfig(*conf)
	if err != nil {
		log.Fatalf("load config error: %s", err.Error())
	}

	syncTask := NewTask(data)
	if data.Cron != "" {
		task := cron.New()
		_, e := task.AddJob(data.Cron, syncTask)
		if e != nil {
			log.Fatalf("add cron task error: %s", e.Error())
		}
		task.Run()
	} else {
		syncTask.Run()
	}
}

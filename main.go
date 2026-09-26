package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"

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
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	data, err := config.NewConfig(ctx, *conf)
	if err != nil {
		log.Fatalf("load config error: %s", err.Error())
	}

	syncTask := NewTask(data)
	if data.Cron != "" {
		syncTask.Interactive = false
		task := cron.New(cron.WithChain(cron.SkipIfStillRunning(cron.DefaultLogger)))
		_, e := task.AddFunc(data.Cron, func() {
			if err := syncTask.Run(ctx); err != nil {
				log.Printf("backup failed: %s", err)
			}
		})
		if e != nil {
			log.Fatalf("add cron task error: %s", e.Error())
		}
		task.Start()
		<-ctx.Done()
		<-task.Stop().Done()
	} else {
		if err := syncTask.Run(ctx); err != nil {
			log.Fatalf("backup failed: %s", err)
		}
	}
}

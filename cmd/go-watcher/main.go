package main

import (
	"fmt"
	"io/ioutil"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/yeqown/go-watcher/internal"
	"github.com/yeqown/go-watcher/utils"

	// TODO: replace log without time and file
	"github.com/silenceper/log"
	"github.com/urfave/cli"
	"gopkg.in/yaml.v1"
)

var (
	exit       chan bool
	paths      []string
	watcher    *internal.Watcher // watcher to watch changed and reload command
	terminated bool              // to control main-goroutine keep running or not
	// cfg            *Config
)

func init() {
	terminated = true
	exit = make(chan bool, 10)
}

func initCommand() *cli.App {
	app := cli.NewApp()

	app.Name = "go-watcher"
	app.Version = "1.1.0"
	app.Author = "yeqown@gmail.com"
	app.Flags = []cli.Flag{}

	app.Commands = []cli.Command{
		{
			Name:  "init",
			Usage: "generate a config file to specified postion",
			Flags: []cli.Flag{
				cli.StringFlag{
					Name:  "output, o",
					Value: "./config.yml",
					Usage: "set target filename of outputing config",
				},
			},
			Action: func(c *cli.Context) error {
				terminated = true
				return generateDefaultConfigFile(c.String("output"))
			},
		},
		{
			Name:  "run",
			Usage: "execute a command, and watch the files, if any change to these files, the command will reload",
			Flags: []cli.Flag{
				cli.StringFlag{
					Name:  "command, e",
					Value: "echo 'go-watcher'",
					Usage: "set command to execute for hot-reloading",
				},
				cli.StringFlag{
					Name:  "config, c",
					Value: "./config.yml",
					Usage: "load configuration from `FILE`, --default=./config.yml",
				},
			},
			Action: func(c *cli.Context) error {
				terminated = false
				var (
					cfg *config
					err error
				)
				// check cfgFile invalid or not
				cfgFilename := c.String("config")
				if !utils.IsFileExist(cfgFilename) {
					return fmt.Errorf("file '%s' is not exist", cfgFilename)
				}
				if cfg, err = loadConfigFile(cfgFilename); err != nil {
					return err
				}

				pwd, _ := os.Getwd()
				// passing config
				paths = append(paths, cfg.additionalPaths...)
				utils.WalkDirectory(pwd, cfg.excludedPaths, &paths, true)
				if watcher, err = internal.
					NewWatcher(paths, exit, []string{"go"}, cfg.excludedRegexps); err != nil {
					return err
				}
				parsed := strings.Split(c.String("command"), " ")
				watcher.SetCommand(parsed[0], parsed[1:], cfg.envs)
				go watcher.Watching() // start watching

				return nil
			},
		},
	}

	return app
}

// [done]: handle command and main goroutine with `terminated` flag
func main() {
	app := initCommand()
	if err := app.Run(os.Args); err != nil {
		log.Fatal(err.Error())
	}

	// by default, terminated is false. it would be set as `true` in app.Commands
	if terminated {
		return
	}

	sigC := make(chan os.Signal)
	signal.Notify(sigC, syscall.SIGKILL, syscall.SIGINT, syscall.SIGQUIT, syscall.SIGHUP)
	go func() {
		for s := range sigC {
			switch s {
			case syscall.SIGINT, syscall.SIGQUIT, syscall.SIGHUP:
				log.Info("sig captured")
				exit <- true
				break
			default:
				time.Sleep(1 * time.Second)
			}
		}
	}()

	for {
		select {
		case <-exit:
			watcher.Exit()
			log.Info("go-watcher exited")
			close(exit)
			os.Exit(2)
		default:
			time.Sleep(3 * time.Second)
			// log.Info("main looping")
		}
	}
}

// config of go-watcher
type config struct {
	excludedRegexps []string `yaml:"excluded_regexps"` // 需要追加监听的文件后缀名字，默认是'.go'，
	additionalPaths []string `yaml:"extern_paths"`     // 额外需要监听的路径
	excludedPaths   []string `yaml:"excluded_paths"`   // 不需要监听的目录
	envs            []string `yaml:"envs"`             // 执行时追加的环境变量
}

func (c *config) String() string {
	return fmt.Sprintf(
		"go-watcher config: \n\texcluded_regexps: %s\n\textern_paths: %s\n\texcluded_paths: %s\n\tenvs: %s\n",
		c.excludedRegexps,
		c.additionalPaths,
		c.excludedPaths,
		c.envs,
	)
}

// loadConfigFile ...
func loadConfigFile(filename string) (cfg *config, err error) {
	var b []byte
	if b, err = ioutil.ReadFile(filename); err == nil {
		if err = yaml.Unmarshal(b, &cfg); err == nil {
			return cfg, nil
		}
	}
	return nil, err
}

// generateDefaultConfigFile ...
func generateDefaultConfigFile(outpath string) error {
	c := &config{
		excludedRegexps: []string{
			".gitignore$",
			".yml$",
			".txt$",
		},
		additionalPaths: []string{},
		envs: []string{
			"GOROOT=/path/to/your/goroot",
			"GOPATH=/path/to/your/gopath",
		},
		excludedPaths: []string{
			"vendor",
		},
	}

	bs, _ := yaml.Marshal(c)
	if err := ioutil.WriteFile(outpath, bs, 0644); err != nil {
		return err
	}
	return nil
}
package main

import (
	"fmt"
	"model"
	"os"
	"runner"
	"runner/sshrunner"
	"time"

	pb "gopkg.in/cheggaaa/pb.v1"

	"utils"

	"github.com/urfave/cli"
)

var (
	// ErrSrcRequired require src option
	ErrSrcRequired = fmt.Errorf("option --src is required")
	// ErrDstRequired require dst option
	ErrDstRequired = fmt.Errorf("option --dst is required")
	// ErrNoNodeToRcp no more node to execute
	ErrNoNodeToRcp = fmt.Errorf("found no node to copy file/directory")
	// ErrLocalPath local path is not existed when put
	ErrLocalPath = fmt.Errorf("local path is not existed")
	// ErrRemotePath remote path is not existed when get
	ErrRemotePath = fmt.Errorf("remote path is not existed")
)

type rcpParams struct {
	GroupName string
	NodeNames []string
	User      string
	Src       string
	Dst       string
	Yes       bool
	PutSize   int64   // size of transfer file/directory for progressbar
	GetSizes  []int64 // size of transfer file/directory for progressbar
}

func initRcpSubCmd(app *cli.App) {

	putSubCmd := cli.Command{
		Name:        "put",
		Usage:       "put <options>",
		Description: "copy file or directory to remote servers",
		Flags: []cli.Flag{
			cli.StringFlag{
				Name:  "src",
				Value: "",
				Usage: "source file or directory",
			},
			cli.StringFlag{
				Name:  "dst",
				Value: "",
				Usage: "destination *directory*",
			},
			cli.StringFlag{
				Name:  "g,group",
				Value: "*",
				Usage: "exec command on one group",
			},
			cli.StringSliceFlag{
				Name:  "n,node",
				Value: &cli.StringSlice{},
				Usage: "exec command on one or more nodes",
			},
			cli.StringFlag{
				Name:  "u,user",
				Value: "root",
				Usage: "user who exec the command",
			},
			cli.BoolFlag{
				Name:  "y,yes",
				Usage: "is confirm before excute command?",
			},
		},
		Action: func(c *cli.Context) error {
			// 如果有 --generate-bash-completion 参数, 则不执行默认命令
			if os.Args[len(os.Args)-1] == "--generate-bash-completion" {
				groupAndNodeComplete(c)
				return nil
			}

			var rp, err = checkRcpParams(c, true)
			if err != nil {
				fmt.Println(utils.FgRed(err))
				cli.ShowCommandHelp(c, "put")
				return nil
			}

			return rcpCmd(rp, true)
		},
	}

	getSubCmd := cli.Command{
		Name:        "get",
		Usage:       "get <options>",
		Description: "copy file or directory from remote servers",
		Flags: []cli.Flag{
			cli.StringFlag{
				Name:  "src",
				Value: "",
				Usage: "source *directory*",
			},
			cli.StringFlag{
				Name:  "dst",
				Value: "",
				Usage: "destination file or directory",
			},
			cli.StringFlag{
				Name:  "g,group",
				Value: "*",
				Usage: "exec command on one group",
			},
			cli.StringSliceFlag{
				Name:  "n,node",
				Value: &cli.StringSlice{},
				Usage: "exec command on one or more nodes",
			},
			cli.StringFlag{
				Name:  "u,user",
				Value: "root",
				Usage: "user who exec the command",
			},
			cli.BoolFlag{
				Name:  "y,yes",
				Usage: "is confirm before excute command?",
			},
		},
		Action: func(c *cli.Context) error {
			// 如果有 --generate-bash-completion 参数, 则不执行默认命令
			if os.Args[len(os.Args)-1] == "--generate-bash-completion" {
				groupAndNodeComplete(c)
				return nil
			}

			var rp, err = checkRcpParams(c, false)
			if err != nil {
				fmt.Println(utils.FgRed(err))
				cli.ShowCommandHelp(c, "get")
				return nil
			}

			return rcpCmd(rp, false)
		},
	}

	if app.Commands == nil {
		app.Commands = cli.Commands{putSubCmd, getSubCmd}
	} else {
		app.Commands = append(app.Commands, putSubCmd, getSubCmd)
	}
}

func checkRcpParams(c *cli.Context, isPut bool) (rcpParams, error) {
	var rp = rcpParams{
		GroupName: c.String("group"),
		NodeNames: c.StringSlice("node"),
		User:      c.String("user"),
		Src:       c.String("src"),
		Dst:       c.String("dst"),
		Yes:       c.Bool("yes"),
	}

	if rp.Dst == "" {
		return rp, ErrSrcRequired
	}

	if rp.Src == "" {
		return rp, ErrSrcRequired
	}

	return rp, nil
}

func rcpCmd(rp rcpParams, isPut bool) error {
	var err error
	// TODO should use sshrunner from config

	// get node info for rcp
	var nodes, _ = repo.FilterNodes(rp.GroupName, rp.NodeNames...)

	if len(nodes) == 0 {
		return ErrNoNodeToRcp
	}

	if !rp.Yes && !confirmRcp(nodes, rp.User, rp.Src, rp.Dst) {
		return nil
	}

	// check file/directory size for put/get
	if isPut {
		rp.PutSize, err = utils.LocalPathSize(rp.Src)
	} else {
		rp.GetSizes, err = getRemotePathSize(nodes, rp.Src)
	}

	if err != nil {
		return err
	}

	// exec cmd on node
	if conf.Main.Sync {
		return concurrentRcp(nodes, rp, isPut)
	} else {
		return syncRcp(nodes, rp, isPut)
	}
	return nil
}

func confirmRcp(nodes []model.Node, user, from, to string) bool {
	fmt.Printf("%-3s\t%-10s\t%-10s\n", "No.", "Name", "Host")
	fmt.Println("----------------------------------------------------------------------")
	for index, n := range nodes {
		fmt.Printf("%-3d\t%-10s\t%-10s\n", index+1, n.Name, n.Host)
	}

	fmt.Println()
	return utils.Confirm(fmt.Sprintf("You want to copy [%s] to [%s] by UESR(%s) at the above nodes, yes/no(y/n) ?",
		utils.FgBoldRed(from), utils.FgBoldRed(to), utils.FgBoldRed(user)))
}

func getRemotePathSize(nodes []model.Node, remotePath string) ([]int64, error) {
	var getSizes = make([]int64, 0)

	for _, n := range nodes {
		var runRcp = sshrunner.New(n.User, n.Password, n.KeyPath, n.Host, n.Port, conf.Main.FileTransBuf)

		var input = runner.RcpInput{
			SrcPath: remotePath,
		}
		size, err := runRcp.RemotePathSize(input)
		if err != nil {
			return getSizes, err
		}
		getSizes = append(getSizes, size)
	}
	return getSizes, nil

}

func syncRcp(nodes []model.Node, rp rcpParams, isPut bool) error {
	var allOutputs = make([]*runner.RcpOutput, 0)
	var rcpStart = time.Now()
	for _, n := range nodes {
		fmt.Printf("%s(%s):\n", utils.FgBoldGreen(n.Name), utils.FgBoldGreen(n.Host))
		var runRcp = sshrunner.New(n.User, n.Password, n.KeyPath, n.Host, n.Port, conf.Main.FileTransBuf)

		var input = runner.RcpInput{
			SrcPath: rp.Src,
			DstPath: rp.Dst,
			RcpHost: n.Host,
			RcpUser: rp.User,
		}

		// display result
		var output *runner.RcpOutput
		if isPut {
			output = runRcp.SyncPut(input)
		} else {
			output = runRcp.SyncGet(input)
		}
		displayRcpResult(output)
		allOutputs = append(allOutputs, output)
	}
	displayTotalRcpResult(allOutputs, rcpStart, time.Now())
	return nil
}

func concurrentRcp(nodes []model.Node, rp rcpParams, isPut bool) error {
	var allOutputs = make([]*runner.RcpOutput, 0)
	var concurrentLimitChan = make(chan int, conf.Main.ConcurrentNum)
	var outputChan = make(chan *runner.ConcurrentRcpOutput)
	var pool *pb.Pool
	pool, _ = pb.StartPool()

	var rcpStart = time.Now()
	for index, n := range nodes {
		var runRcp = sshrunner.New(n.User, n.Password, n.KeyPath, n.Host, n.Port, conf.Main.FileTransBuf)
		var input = runner.RcpInput{
			SrcPath: rp.Src,
			DstPath: rp.Dst,
			RcpHost: n.Host,
			RcpUser: rp.User,
		}

		// rcp file/directory
		if isPut {
			input.RcpSize = rp.PutSize
			go runRcp.ConcurrentPut(input, outputChan, concurrentLimitChan, pool)
		} else {
			input.RcpSize = rp.GetSizes[index]
			go runRcp.ConcurrentGet(input, outputChan, concurrentLimitChan, pool)
		}
	}

	var totalCnt = len(nodes)
	for ch := range outputChan {
		totalCnt -= 1
		// fmt.Printf("RCP by %s(%s):\n", utils.FgBoldGreen(ch.In.RcpUser), utils.FgBoldGreen(ch.In.RcpHost))
		// displayRcpResult(ch.Out)
		allOutputs = append(allOutputs, ch.Out)

		if totalCnt == 0 {
			close(outputChan)
		}
	}

	pool.Stop()
	fmt.Println(utils.FgBoldBlue("==========================================================\n"))
	displayTotalRcpResult(allOutputs, rcpStart, time.Now())
	return nil
}

func displayRcpResult(output *runner.RcpOutput) {
	if output.Err != nil {
		fmt.Printf("copy file/directory failed: %s\n", utils.FgRed(output.Err))
	}

	fmt.Printf("time costs: %v\n", output.RcpEnd.Sub(output.RcpStart))
	fmt.Println(utils.FgBoldBlue("==========================================================\n"))
}

func displayTotalRcpResult(outputs []*runner.RcpOutput, rcpStart, rcpEnd time.Time) {
	var successCnt, failCnt int

	for _, output := range outputs {
		if output.Err != nil {
			failCnt += 1
		} else {
			successCnt += 1
		}
	}

	fmt.Printf("total time costs: %v\nRCP success nodes: %s | fail nodes: %s\n\n\n",
		rcpEnd.Sub(rcpStart),
		utils.FgBoldGreen(successCnt),
		utils.FgBoldRed(failCnt))
}

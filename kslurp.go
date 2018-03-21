package main

import (
	"bufio"
	"crypto/rand"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"strings"
)

type Control struct {
	cmds chan *exec.Cmd
	stop chan string
}

type Pod struct {
	name      string
	container string
	color     int
}

var ctx *string
var width int

func colorize(s string, c int) string {
	return fmt.Sprintf("\033[38;5;%dm%s\033[0m", c, s)
}

func randomColor() int {
	r := make([]byte, 1)
	l, _ := rand.Read(r)
	if l == len(r) {
		return int(r[0])%193 + 30
	}
	return 4
}

func (p *Pod) Name() string {
	return colorize(p.name, p.color)
}

func (p *Pod) WideName() string {
	return colorize(fmt.Sprintf("%-[2]*[1]s", p.name, width), p.color)
}

func (p *Pod) Subscribe(ctrl *Control, log chan string) {
	fmt.Println("Subscribed to", p.Name())

	cmd := kubeExec("log", "--tail", "10", "--follow", p.name, p.container)
	ctrl.cmds <- cmd

	stdout, _ := cmd.StdoutPipe()
	scanner := bufio.NewScanner(stdout)

	go func() {
		for scanner.Scan() {
			log <- fmt.Sprintf("[%s] %s", p.WideName(), scanner.Text())
		}
	}()

	cmd.Start()
	cmd.Wait()
	ctrl.stop <- p.Name()
}

func kubeExec(args ...string) *exec.Cmd {
	if len(*ctx) > 0 {
		args = append(args, "--context", *ctx)
	}
	cmd := exec.Command("kubectl", args...)
	return cmd
}

func pods(selector *string) []*Pod {
	jsp := "jsonpath={range .items[*]}{.metadata.name} {.spec.containers[0].name}{\"\\n\"}{end}"
	cmd := kubeExec("get", "pod", "--output", jsp, "--selector", *selector)
	out, err := cmd.CombinedOutput()
	if err != nil {
		fmt.Printf("kubectl (%s): %s", err, string(out))
		os.Exit(1)
	}
	kpods := strings.Split(strings.Trim(string(out), " \n"), "\n")

	width = 0
	pods := make([]*Pod, 0)

	for _, p := range kpods {
		s := strings.SplitN(p, " ", 2)
		l := len(s[0])
		if l > width {
			width = l
		}
		pods = append(pods, &Pod{name: s[0], container: s[1], color: randomColor()})
	}

	return pods
}

func StopHandler(ctrl *Control) {
	name := <-ctrl.stop
	fmt.Println("Child process terminated:", name)
	for cmd := range ctrl.cmds {
		cmd.Process.Kill()
		cmd.Wait()
		if len(ctrl.cmds) == 0 {
			break
		}
	}
	os.Exit(0)
}

func main() {
	ctx = flag.String("context", "", "The name of the kubeconfig context to use")
	selector := flag.String("l", "", "Selector (label query) to filter on, supports '=', '==', and '!='.")
	flag.Parse()

	pods := pods(selector)
	log := make(chan string)
	ctrl := &Control{cmds: make(chan *exec.Cmd, len(pods)), stop: make(chan string)}

	for _, pod := range pods {
		go pod.Subscribe(ctrl, log)
	}

	go StopHandler(ctrl)

	for {
		fmt.Println(<-log)
	}
}

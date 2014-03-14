/*
 *  docker-run-app      run an arbitrary command and forward signals to said command.
 *  Copyright (c) 2014 Justin Charette <charetjc@gmail.com> (@boxofrox)
 *                All Rights Reserved
 *
 *  This program is free software. It comes without any warranty, to
 *  the extent permitted by applicable law. You can redistribute it
 *  and/or modify it under the terms of the Do What the Fuck You Want
 *  to Public License, Version 2, as published by Sam Hocevar. See
 *  http://www.wtfpl.net/ for more details.
 *
 *
 * Usage:     docker-run-app [-h] [--init-log FILE] [--] COMMAND
 *
 *   COMMAND         - app and args to execute. app requires full path.
 *   --              - args after this flag are reserved for COMMAND.
 *   -h, --help      - print this help message.
 *   --init-log FILE - write docker-run-app output to FILE.
 *   -V, --version   - print version info.
 */
package main

import (
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"os/signal"
	"path"
	"strings"
	"syscall"
	"time"
)

var (
	VERSION    string
	BUILD_DATE string
)

const (
	SIG_TIMEOUT = time.Second * 2
)

const (
	OK AppError = iota
	AppStoppedWithError
	CannotStartApp
	FailedToKillApp
	MissingArgument
	InsufficientSignalError
	InvalidCommand
	BadFlag
)

const (
	FlagFound FlagError = iota
	FlagNotFound
	FlagHasTooFewParams
)

type AppError int
type FlagError int
type ParamList []string

func main() {
	var (
		args    []string
		err     AppError = OK
		file    *os.File
		fileErr error
		options map[string]string
	)

	options, args = parseFlags(os.Args[1:])

	if options["init-log"] != "" {
		if file, fileErr = os.OpenFile(options["init-log"], os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0664); fileErr != nil {
			log.Printf("Cannot open log file (%s).  Using stderr.", options["init-log"])
		} else {
			log.SetOutput(file)
		}
	}

	// has command?
	if len(args) == 0 {
		usage()
		log.Println("missing <command>. ")
		err = MissingArgument
	} else {
		cmd := args[0]

		if len(args) > 1 {
			args = args[1:]
		} else {
			args = nil
		}

		err = runCommand(exec.Command(cmd, args...))
	}

	if file != nil {
		file.Close()
	}

	os.Exit(int(err))
}

// eatFlag
//
//  Search argument array for one flag and possibly one or more parameters.
//  The flag can be one or more representations of the same flag (e.g. -h, --help).
//  return the file and the remaining options in a new array.
//
func eatFlag(args []string, flags []string, paramCount int) (params ParamList, remaining []string, err FlagError) {
	var (
		a, b int
	)

	hasFlag := func(arg string) bool {
		for i := range flags {
			if flags[i] == arg {
				return true
			}
		}
		return false
	}

	if len(args) == 0 {
		params, remaining, err = ParamList{}, args, FlagNotFound
		return
	}

	remaining = make([]string, len(args))
	params = ParamList{}
	err = FlagNotFound

ArgLoop:
	for a, b = 0, 0; a < len(args); a++ {
		if "--" == args[a] {
			// stop processing flags
			a++
			break ArgLoop
		} else if hasFlag(args[a]) {
			a++
			availCount := len(args) - a // make sure our attempt to grab parameters does not exceeed arg array bounds.

			// can we eat all the params this flag needs?
			if availCount < paramCount {
				params, remaining, err = ParamList{}, args, FlagHasTooFewParams
				return
			}

			params = make(ParamList, paramCount)

			for c := 0; c < availCount; c++ {
				// only eat params, don't eat potential flags
				if !strings.HasPrefix(args[a], "-") {
					params[c] = args[a]
					a++
				} else {
					params, remaining, err = ParamList{}, args, FlagHasTooFewParams
					return
				}
			}

			err = FlagFound
			break ArgLoop

		} else {
			// copy unused arguments
			remaining[b] = args[a]
			b++
		}
	}

	// copy remaining arguments
	for ; a < len(args); a++ {
		remaining[b] = args[a]
		b++
	}

	return
}

func envOr(name string, def string) string {
	if val := os.Getenv(name); val != "" {
		return val
	}
	return def
}

func parseFlags(args []string) (options map[string]string, remaining []string) {
	var (
		flagErr FlagError
		params  ParamList
	)

	remaining = args

	// VERSION. eat flag. exit if found.
	if _, remaining, flagErr = eatFlag(remaining, []string{"-V", "--version"}, 0); flagErr == FlagFound {
		version()
		os.Exit(int(OK))
	}

	// HELP. eat flag. exit if found.
	if _, remaining, flagErr = eatFlag(remaining, []string{"-h", "--help"}, 0); flagErr == FlagFound {
		usage()
		os.Exit(int(OK))
	}

	// we now have potential flags to return

	options = make(map[string]string)

	// INIT LOG. eat flag, 1 param. exit if error.
	if params, remaining, flagErr = eatFlag(remaining, []string{"--init-log"}, 1); flagErr == FlagHasTooFewParams {
		log.Println("Error: flag --init-log is missing an argument.")
		usage()
		os.Exit(int(BadFlag))
	} else {
		options["init-log"] = params.getOr(0, "")
	}

	return
}

func runCommand(cmd *exec.Cmd) AppError {
	sigs := make(chan os.Signal, 1)
	done := make(chan error, 1)

	// listen for signals from docker daemon
	signal.Notify(sigs, syscall.SIGINT, syscall.SIGTERM)

	// run the app from goroutine, so we can monitor signals and app
	// termination
	go func() {
		stdout, err := cmd.StdoutPipe()
		if err != nil {
			log.Println("Cannot open pipe to app's stdout: ", err)
		}

		stderr, err := cmd.StderrPipe()
		if err != nil {
			log.Println("Cannot open pipe to app's stderr: ", err)
		}

		err = cmd.Start()
		if err != nil {
			log.Fatal(err)
			done <- err
			return
		}

		log.Println("App started.")

		// redirect apps's stdout/stderr to our stdout/stderr, respectively
		go io.Copy(os.Stdout, stdout)
		go io.Copy(os.Stderr, stderr)

		err = cmd.Wait()
		done <- err
	}()

	// monitor termination of app or signals from docker
	select {
	case err := <-done:
		if err == nil {
			log.Println("App stopped.")
			return OK
		} else {
			log.Printf("App stopped with error (%v)", err)
			return AppStoppedWithError
		}
	case sig := <-sigs:
		log.Printf("Received signal (%v).", sig)

		sigSuccess, err := stopProcess(cmd.Process, sig, syscall.SIGTERM, syscall.SIGHUP)

		if err != OK {
			log.Println(err)
			return err
		}

		log.Printf("App stopped with signal (%v).\n", sigSuccess)

		// did app stop with the expected signal?
		switch sigSuccess {
		case sig:
			return OK
		case syscall.SIGINT:
			return OK
		default:
			return InsufficientSignalError
		}
	}

	return OK
}

/** stopProcess
 *
 * given a process and an ordered list of signals, send the first signal and
 * delay.  if the process did not stop, then repeat with subsequent signals
 * until the app responds, or we run out of signals.
 */
func stopProcess(p *os.Process, sigs ...os.Signal) (os.Signal, AppError) {
	if len(sigs) == 0 {
		if err := p.Kill(); err != nil {
			log.Fatal("Failed to kill app: ", err)
		}
		return nil, FailedToKillApp
	}

	c := make(chan error, 1)

	go func() {
		log.Printf("Attempting to stop app with signal (%v).", sigs[0])
		c <- p.Signal(sigs[0])
	}()

	select {
	case err := <-c:
		if err == nil {
			return sigs[0], OK
		} else {
			return stopProcess(p, sigs[1:]...)
		}
	case _ = <-time.After(SIG_TIMEOUT):
		return stopProcess(p, sigs[1:]...)
	}
}

func usage() {
	prog := path.Base(os.Args[0])

	fmt.Printf("Usage:     %s [-h] [--init-log FILE] [--] COMMAND\n", prog)
	fmt.Println()
	fmt.Println("Commands:")
	fmt.Println()
	fmt.Println("  COMMAND         - app and args to execute. app requires full path.")
	fmt.Println("  --              - args after this flag are reserved for COMMAND.")
	fmt.Println("  -h, --help      - print this help message.")
	fmt.Printf("  --init-log FILE - write %s output to FILE.\n", prog)
	fmt.Println("  -V, --version   - print version info.")
	fmt.Println()
}

func version() {
	fmt.Printf("%s: version %s, build %s\n", os.Args[0], VERSION, BUILD_DATE)
	fmt.Println()
}

func (err AppError) Error() string {
	switch err {
	case MissingArgument:
		return "missing argument"
	case InsufficientSignalError:
		return "SIGINT insufficient to stop app"
	default:
		return "unknown error"
	}
}

func (err FlagError) Error() string {
	switch err {
	case FlagFound:
		return "flag found"
	case FlagNotFound:
		return "flag not found"
	case FlagHasTooFewParams:
		return "flag is missing required parameters"
	default:
		return "unknown error"
	}
}

func (p *ParamList) getOr(index int, def string) string {
	if index < 0 || index >= len(*p) {
		return def
	}

	return (*p)[index]
}

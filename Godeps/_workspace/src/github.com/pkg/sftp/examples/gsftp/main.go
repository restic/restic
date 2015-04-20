// gsftp implements a simple sftp client.
//
// gsftp understands the following commands:
//
// List a directory (and its subdirectories)
//      gsftp ls DIR
//
// Fetch a remote file
//      gsftp fetch FILE
//
// Put the contents of stdin to a remote file
//      cat LOCALFILE | gsftp put REMOTEFILE
//
// Print the details of a remote file
//      gsftp stat FILE
//
// Remove a remote file
//      gsftp rm FILE
//
// Rename a file
//      gsftp mv OLD NEW
//
package main

import (
	"flag"
	"fmt"
	"io"
	"log"
	"net"
	"os"

	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/agent"

	"github.com/pkg/sftp"
)

var (
	USER = flag.String("user", os.Getenv("USER"), "ssh username")
	HOST = flag.String("host", "localhost", "ssh server hostname")
	PORT = flag.Int("port", 22, "ssh server port")
	PASS = flag.String("pass", os.Getenv("SOCKSIE_SSH_PASSWORD"), "ssh password")
)

func init() {
	flag.Parse()
	if len(flag.Args()) < 1 {
		log.Fatal("subcommand required")
	}
}

func main() {
	var auths []ssh.AuthMethod
	if aconn, err := net.Dial("unix", os.Getenv("SSH_AUTH_SOCK")); err == nil {
		auths = append(auths, ssh.PublicKeysCallback(agent.NewClient(aconn).Signers))

	}
	if *PASS != "" {
		auths = append(auths, ssh.Password(*PASS))
	}

	config := ssh.ClientConfig{
		User: *USER,
		Auth: auths,
	}
	addr := fmt.Sprintf("%s:%d", *HOST, *PORT)
	conn, err := ssh.Dial("tcp", addr, &config)
	if err != nil {
		log.Fatalf("unable to connect to [%s]: %v", addr, err)
	}
	defer conn.Close()

	client, err := sftp.NewClient(conn)
	if err != nil {
		log.Fatalf("unable to start sftp subsytem: %v", err)
	}
	defer client.Close()
	switch cmd := flag.Args()[0]; cmd {
	case "ls":
		if len(flag.Args()) < 2 {
			log.Fatalf("%s %s: remote path required", cmd, os.Args[0])
		}
		walker := client.Walk(flag.Args()[1])
		for walker.Step() {
			if err := walker.Err(); err != nil {
				log.Println(err)
				continue
			}
			fmt.Println(walker.Path())
		}
	case "fetch":
		if len(flag.Args()) < 2 {
			log.Fatalf("%s %s: remote path required", cmd, os.Args[0])
		}
		f, err := client.Open(flag.Args()[1])
		if err != nil {
			log.Fatal(err)
		}
		defer f.Close()
		if _, err := io.Copy(os.Stdout, f); err != nil {
			log.Fatal(err)
		}
	case "put":
		if len(flag.Args()) < 2 {
			log.Fatalf("%s %s: remote path required", cmd, os.Args[0])
		}
		f, err := client.Create(flag.Args()[1])
		if err != nil {
			log.Fatal(err)
		}
		defer f.Close()
		if _, err := io.Copy(f, os.Stdin); err != nil {
			log.Fatal(err)
		}
	case "stat":
		if len(flag.Args()) < 2 {
			log.Fatalf("%s %s: remote path required", cmd, os.Args[0])
		}
		f, err := client.Open(flag.Args()[1])
		if err != nil {
			log.Fatal(err)
		}
		defer f.Close()
		fi, err := f.Stat()
		if err != nil {
			log.Fatalf("unable to stat file: %v", err)
		}
		fmt.Printf("%s %d %v\n", fi.Name(), fi.Size(), fi.Mode())
	case "rm":
		if len(flag.Args()) < 2 {
			log.Fatalf("%s %s: remote path required", cmd, os.Args[0])
		}
		if err := client.Remove(flag.Args()[1]); err != nil {
			log.Fatalf("unable to remove file: %v", err)
		}
	case "mv":
		if len(flag.Args()) < 3 {
			log.Fatalf("%s %s: old and new name required", cmd, os.Args[0])
		}
		if err := client.Rename(flag.Args()[1], flag.Args()[2]); err != nil {
			log.Fatalf("unable to rename file: %v", err)
		}
	default:
		log.Fatalf("unknown subcommand: %v", cmd)
	}
}

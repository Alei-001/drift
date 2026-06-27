package app

import (
	"bufio"
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/drift/drift/internal/config"
	"golang.org/x/term"
)

func (a *App) RemoteSetup() error {
	gcfg, err := config.LoadGlobalConfig()
	if err != nil {
		return err
	}

	reader := bufio.NewReader(os.Stdin)

	fmt.Print("Protocol (webdav/ftp/sftp/smb): ")
	proto, _ := reader.ReadString('\n')
	proto = strings.TrimSpace(proto)
	if proto != "" {
		gcfg.Protocol = proto
	}

	fmt.Print("Host: ")
	host, _ := reader.ReadString('\n')
	host = strings.TrimSpace(host)
	if host != "" {
		gcfg.Host = host
	}

	fmt.Print("Port (0=default): ")
	portStr, _ := reader.ReadString('\n')
	portStr = strings.TrimSpace(portStr)
	if portStr != "" && portStr != "0" {
		if port, err := strconv.Atoi(portStr); err == nil {
			gcfg.Port = port
		}
	}

	fmt.Print("Path: ")
	path, _ := reader.ReadString('\n')
	path = strings.TrimSpace(path)
	if path != "" {
		gcfg.Path = path
	}

	fmt.Print("Username: ")
	user, _ := reader.ReadString('\n')
	user = strings.TrimSpace(user)
	if user != "" {
		gcfg.Username = user
	}

	fmt.Print("Password: ")
	passBytes, err := term.ReadPassword(int(os.Stdin.Fd()))
	fmt.Println()
	var pass string
	if err == nil {
		pass = strings.TrimSpace(string(passBytes))
	}
	if pass != "" {
		gcfg.Password = pass
	}

	fmt.Print("TLS? (y/N): ")
	tlsStr, _ := reader.ReadString('\n')
	tlsStr = strings.TrimSpace(tlsStr)
	if strings.EqualFold(tlsStr, "y") || strings.EqualFold(tlsStr, "yes") {
		gcfg.TLS = true
	}

	if err := config.SaveGlobalConfig(gcfg); err != nil {
		return err
	}

	fmt.Printf("Remote saved: %s://%s%s\n", gcfg.Protocol, gcfg.Host, gcfg.Path)
	return nil
}

func (a *App) RemoteShow() error {
	gcfg, err := config.LoadGlobalConfig()
	if err != nil {
		return err
	}

	if gcfg.Protocol == "" {
		fmt.Println("No remote configured. Use 'drift remote setup' to configure.")
		return nil
	}

	fmt.Printf("Protocol:  %s\n", gcfg.Protocol)
	fmt.Printf("Host:      %s\n", gcfg.Host)
	if gcfg.Port != 0 {
		fmt.Printf("Port:      %d\n", gcfg.Port)
	}
	fmt.Printf("Path:      %s\n", gcfg.Path)
	fmt.Printf("Username:  %s\n", gcfg.Username)
	fmt.Printf("TLS:       %v\n", gcfg.TLS)
	if gcfg.Protocol == "smb" && gcfg.Share != "" {
		fmt.Printf("Share:     %s\n", gcfg.Share)
	}
	if gcfg.Protocol == "sftp" && gcfg.KeyPath != "" {
		fmt.Printf("Key path:  %s\n", gcfg.KeyPath)
	}
	return nil
}

func (a *App) RemoteRemove() error {
	gcfg, err := config.LoadGlobalConfig()
	if err != nil {
		return err
	}

	gcfg.Protocol = ""
	gcfg.Host = ""
	gcfg.Port = 0
	gcfg.Path = ""
	gcfg.Username = ""
	gcfg.Password = ""
	gcfg.TLS = false
	gcfg.InsecureSkipVerify = false
	gcfg.Share = ""
	gcfg.KeyPath = ""

	if err := config.SaveGlobalConfig(gcfg); err != nil {
		return err
	}

	fmt.Println("Remote configuration removed")
	return nil
}

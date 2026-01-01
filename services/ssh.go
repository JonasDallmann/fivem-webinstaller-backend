package services

import (
	"fmt"
	"io"
	"log"

	"golang.org/x/crypto/ssh"
)

type SSHClient struct {
	Config *ssh.ClientConfig
	Host   string
	Port   int
}

func (s *SSHClient) RunScript(scriptContent string, args string) (string, error) {
	addr := fmt.Sprintf("%s:%d", s.Host, s.Port)
	client, err := ssh.Dial("tcp", addr, s.Config)
	if err != nil {
		return "", fmt.Errorf("could not connect to SSH: %v", err)
	}
	defer func(client *ssh.Client) {
		err := client.Close()
		if err != nil {
			log.Printf("Error closing SSH client: %v", err)
		}
	}(client)

	session, err := client.NewSession()
	if err != nil {
		return "", fmt.Errorf("SSH session error: %v", err)
	}
	defer func(session *ssh.Session) {
		err := session.Close()
		if err != nil {
			log.Printf("Error closing SSH session: %v", err)
		}
	}(session)

	remoteCmd := fmt.Sprintf("cat > /tmp/fivem_setup.sh && chmod +x /tmp/fivem_setup.sh && /tmp/fivem_setup.sh %s", args)

	stdin, _ := session.StdinPipe()
	go func() {
		defer func(stdin io.WriteCloser) {
			err := stdin.Close()
			if err != nil {

			}
		}(stdin)
		_, err := io.WriteString(stdin, scriptContent)
		if err != nil {
			return
		}
	}()

	output, err := session.CombinedOutput(remoteCmd)

	if err != nil {
		return string(output), fmt.Errorf("installation error: %v", err)
	}

	return string(output), nil
}

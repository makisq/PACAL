package main

import (
	"bufio"
	"crypto/tls"
	"fmt"
	"net"
	"strings"
	"sync"
)

var (
	connMap = &sync.Map{}
)

func StartServer() {
	cert, err := tls.LoadX509KeyPair("server.crt", "server.key")
	if err != nil {
		fmt.Println("Ошибка загрузки сертификатов:", err)
		return
	}
	config := &tls.Config{Certificates: []tls.Certificate{cert}}

	l, err := tls.Listen("tcp", "localhost:8080", config)
	if err != nil {
		fmt.Println("Ошибка запуска сервера:", err)
		return
	}
	defer l.Close()
	fmt.Println("Защищенный сервер слушает на localhost:8080 (TLS)")

	for {
		conn, err := l.Accept()
		if err != nil {
			fmt.Println("Ошибка подключения:", err)
			continue
		}
		go handleConnection(conn)
	}
}

func handleConnection(c net.Conn) {
	nick, err := getNickname(c)
	if err != nil {
		c.Close()
		return
	}

	if _, loaded := connMap.LoadOrStore(nick, c); loaded {
		c.Write([]byte("Этот ник уже занят. Попробуйте другой.\n"))
		c.Close()
		return
	}

	defer disconnectClient(nick, c)

	c.Write([]byte(fmt.Sprintf("Добро пожаловать, %s! Ваши сообщения будут видны другим.\n", nick)))
	broadcast(nick, fmt.Sprintf("%s присоединился к чату\n", nick), false)

	reader := bufio.NewReader(c)
	for {
		message, err := reader.ReadString('\n')
		if err != nil {
			break
		}

		message = strings.TrimSpace(message)

		if strings.HasPrefix(message, "/") {
			handleCommand(nick, c, message)
			continue
		}

		broadcast(nick, fmt.Sprintf("[%s]: %s\n", nick, message), true)
	}
}

func getNickname(c net.Conn) (string, error) {
	c.Write([]byte("Введите ваш ник: "))
	nick, err := bufio.NewReader(c).ReadString('\n')
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(nick), nil
}

func disconnectClient(nick string, c net.Conn) {
	c.Close()
	connMap.Delete(nick)
	broadcast(nick, fmt.Sprintf("%s покинул чат\n", nick), false)
	fmt.Printf("Клиент %s отключился\n", nick)
}

func broadcast(sender string, message string, includeSender bool) {
	connMap.Range(func(key, value interface{}) bool {
		if nick, ok := key.(string); ok {
			if conn, ok := value.(net.Conn); ok {
				if includeSender || nick != sender {
					conn.Write([]byte(message))
				}
			}
		}
		return true
	})
}

func handleCommand(nick string, c net.Conn, cmd string) {
	switch {
	case strings.HasPrefix(cmd, "/file"):
		c.Write([]byte("Функция загрузки файлов в разработке\n"))
	case strings.HasPrefix(cmd, "/pm"):
		parts := strings.SplitN(cmd, " ", 3)
		if len(parts) < 3 {
			c.Write([]byte("Использование: /pm [ник] [сообщение]\n"))
			return
		}
		targetNick := parts[1]
		if value, ok := connMap.Load(targetNick); ok {
			if targetConn, ok := value.(net.Conn); ok {
				targetConn.Write([]byte(fmt.Sprintf("[ЛС от %s]: %s\n", nick, parts[2])))
				c.Write([]byte(fmt.Sprintf("[ЛС для %s]: %s\n", targetNick, parts[2])))
			}
		} else {
			c.Write([]byte("Пользователь не найден\n"))
		}
	default:
		c.Write([]byte("Неизвестная команда\n"))
	}
}

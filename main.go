package main

import (
	"crypto/rand"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"io/ioutil"
	"log"
	"net"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
)

type Entity struct {
	ID    int
	Pos   [3]float64 // X, Y, Z
	State string
}

type ClientState struct {
	addr              *net.UDPAddr
	readyForSnapshots bool
}

type ServerConfig struct {
	Map        string
	MaxPlayers int
	ServerName string
	GameType   string
	MapRotate  []string
}

var serverConfig ServerConfig

var clients = make(map[string]*ClientState)

func main() {

	if err := loadGameConfigs("./config"); err != nil {
		log.Fatalf("Error when loading configuration: %v", err)
	}

	addr := net.UDPAddr{
		Port: 28960,
		IP:   net.ParseIP("0.0.0.0"),
	}
	conn, err := net.ListenUDP("udp", &addr)
	if err != nil {
		log.Fatalf("Error when creating UDP server: %v", err)
	}
	defer conn.Close()

	log.Printf("UDP serveur started on %v", conn.LocalAddr())

	clientAddresses := make([]*net.UDPAddr, 0, len(clients))
	for _, client := range clients {
		if client.readyForSnapshots {
			clientAddresses = append(clientAddresses, client.addr)
		}
	}

	entities := []Entity{
		{ID: 1, Pos: [3]float64{100.0, 200.0, 300.0}, State: "active"},
	}

	go func() {
		clientAddresses := make([]*net.UDPAddr, 0, len(clients))
		for _, client := range clients {
			clientAddresses = append(clientAddresses, client.addr)
		}
		sendRegularSnapshots(conn, clientAddresses, entities)
	}()

	for {
		handleClient(conn)
	}
}

func loadServerConfig(configPath string) (ServerConfig, error) {
	var config ServerConfig

	content, err := ioutil.ReadFile(configPath)
	if err != nil {
		return config, err
	}

	lines := strings.Split(string(content), "\n")
	for _, line := range lines {
		parts := strings.SplitN(line, ": ", 2)
		if len(parts) != 2 {
			continue
		}

		key, value := parts[0], parts[1]
		switch key {
		case "map":
			config.Map = value
		case "maxPlayers":
			config.MaxPlayers, _ = strconv.Atoi(value)
		case "serverName":
			config.ServerName = value
		case "gameType":
			config.GameType = value
		case "mapRotate":
			config.MapRotate = strings.Split(value, ", ")
		}
	}

	return config, nil
}

func loadGameConfigs(configPath string) error {
	files, err := ioutil.ReadDir(configPath)
	if err != nil {
		return err
	}

	for _, file := range files {
		filePath := filepath.Join(configPath, file.Name())
		_, err := ioutil.ReadFile(filePath)
		if err != nil {
			log.Printf("Error when reading file %s: %v", filePath, err)
			continue
		}
		log.Printf("Config sucessfully loaded %s", filePath)
	}

	return nil
}

func handleGetChallenge(conn *net.UDPConn, clientAddr *net.UDPAddr) {
	challengeResponse := generateChallengeResponse()
	_, err := conn.WriteToUDP([]byte(challengeResponse), clientAddr)
	if err != nil {
		log.Printf("Erreur when sending challenge response: %v", err)
	} else {
		log.Printf("Response challenge successfully sent%v", clientAddr)
	}
}

func handleConnectRequest(conn *net.UDPConn, clientAddr *net.UDPAddr, receivedData string) {
	log.Printf("Error when connecting %v", clientAddr)
	connectResponse := "\xff\xff\xff\xffconnectResponse\n"
	connectResponse += "sessionid=unique_session_id;map=nom_de_la_carte;status=ok;"
	_, err := conn.WriteToUDP([]byte(connectResponse), clientAddr)
	if err != nil {
		log.Printf("Error when sending connexion response: %v", err)
		return
	}
	log.Println("Success sending connexion")

	clientKey := clientAddr.String()
	client, exists := clients[clientKey]
	if !exists {
		client = &ClientState{addr: clientAddr, readyForSnapshots: false}
		clients[clientKey] = client
	}

	sendConfigStrings(conn, clientAddr)

	sendGameState(conn, clientAddr, client)
}

func generateChallengeResponse() string {
	var b [8]byte
	_, err := rand.Read(b[:])
	if err != nil {
		log.Fatal(err)
	}
	token := binary.BigEndian.Uint64(b[:])
	return fmt.Sprintf("\xff\xff\xff\xffchallengeResponse %d", token)
}

func isAckPacket(data string) bool {
	return strings.Contains(data, "ack")
}

func handleAck(conn *net.UDPConn, clientAddr *net.UDPAddr) {
	log.Println("ACK reçu du client, envoi de l'état du jeu...")

	gameState := "\xff\xff\xff\xffgamestate map=mp_harbor;gametype=dm;"
	/*	gameState += "map=mp_harbor;gametype=dm;" */
	_, err := conn.WriteToUDP([]byte(gameState), clientAddr)
	if err != nil {
		log.Printf("Error when sending gamestate: %v", err)
		runtime.Breakpoint()
	} else {
		log.Println("handleAck : Gamestate sent to client")
		clientKey := clientAddr.String()
		if client, exists := clients[clientKey]; exists {
			client.readyForSnapshots = true
		} else {
			clients[clientKey] = &ClientState{addr: clientAddr, readyForSnapshots: true}
		}
	}
}

func sendGameState(conn *net.UDPConn, clientAddr *net.UDPAddr, client *ClientState) {
	log.Println("Sending gamestate to the client", clientAddr)

	sendConfigStrings(conn, clientAddr)

	log.Println("Sending baseline to the client", clientAddr)

	client.readyForSnapshots = true
}

func handleStatusUpdate(conn *net.UDPConn, clientAddr *net.UDPAddr, receivedData string) {
	sequenceNumber, err := parseSequenceNumber(receivedData)
	if err != nil {
		log.Printf("Error: %v", err)
		return
	}

	log.Printf("Mise à jour d'état reçue de %v avec numéro de séquence: %d", clientAddr, sequenceNumber)
}

func parseSequenceNumber(data string) (int, error) {
	var sequenceNumber int
	if len(data) < 4 {
		return 0, fmt.Errorf("paquet trop court")
	}
	sequenceNumber = int(binary.BigEndian.Uint32([]byte(data[0:4])))
	return sequenceNumber, nil
}

func sendConfigStrings(conn *net.UDPConn, clientAddr *net.UDPAddr) {
	configStrings := []string{
		"mapname mp_harbor",
		"gametype dm",
		// Ajoutez d'autres configStrings ici
	}
	for _, configString := range configStrings {
		packet := fmt.Sprintf("\xff\xff\xff\xffconfigString %s\n", configString)
		if _, err := conn.WriteToUDP([]byte(packet), clientAddr); err != nil {
			log.Printf("Error when sending configstring to the client %v: %v", clientAddr, err)
		}
	}
}

func handleClient(conn *net.UDPConn) {
	buffer := make([]byte, 1024)
	n, clientAddr, err := conn.ReadFromUDP(buffer)
	if err != nil {
		log.Printf("Error data client: %v", err)
		return
	}

	log.Printf("Recipe %d octets from %v: %s", n, clientAddr, hex.EncodeToString(buffer[:n]))

	receivedData := string(buffer[:n])
	if strings.HasPrefix(receivedData, "\xff\xff\xff\xffgetchallenge") {
		handleGetChallenge(conn, clientAddr)
	} else if strings.HasPrefix(receivedData, "\xff\xff\xff\xffconnect") && n >= 136 {
		handleConnectRequest(conn, clientAddr, receivedData)
	} else if strings.HasPrefix(receivedData, "\xff\xff\xff\xffack") {
		handleAck(conn, clientAddr)
	} else if len(receivedData) == 16 {
		handle16BytePacket(conn, clientAddr, buffer[:n])
	} else {
		handleStatusUpdate(conn, clientAddr, receivedData)
	}
}

func handle16BytePacket(conn *net.UDPConn, clientAddr *net.UDPAddr, data []byte) {
	sequenceNumber := binary.BigEndian.Uint32(data[:4])
	log.Printf("Paquet de 16 octets reçu de %v avec numéro de séquence: %d", clientAddr, sequenceNumber)

}

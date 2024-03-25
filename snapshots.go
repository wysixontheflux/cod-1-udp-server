package main

import (
	"fmt"
	"log"
	"net"
	"time"
)

func generateSnapshot(entities []Entity) []byte {
	var snapshot []byte
	header := []byte("COD1GameState|")
	snapshot = append(snapshot, header...)
	for _, e := range entities {
		entitySnapshot := fmt.Sprintf("EntityID:%d|PosX:%.2f|PosY:%.2f|PosZ:%.2f|State:%s|", e.ID, e.Pos[0], e.Pos[1], e.Pos[2], e.State)
		snapshot = append(snapshot, entitySnapshot...)
	}
	return snapshot
}

func sendSnapshotsToClient(conn *net.UDPConn, clientAddr *net.UDPAddr, entities []Entity) {
	snapshot := generateSnapshot(entities)
	packet := append([]byte("\xff\xff\xff\xffsnapshot\n"), snapshot...)
	if _, err := conn.WriteToUDP(packet, clientAddr); err != nil {
		log.Printf("Error when sending snapshot to the client %v: %v", clientAddr, err)
	}
}

func sendRegularSnapshots(conn *net.UDPConn, clients []*net.UDPAddr, entities []Entity) {
	for {
		for _, clientAddr := range clients {
			sendSnapshotsToClient(conn, clientAddr, entities)
		}
		time.Sleep(50 * time.Millisecond)
	}
}

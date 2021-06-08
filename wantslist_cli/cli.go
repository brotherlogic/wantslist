package main

import (
	"bufio"
	"fmt"
	"log"
	"os"
	"strconv"
	"strings"

	"github.com/brotherlogic/goserver/utils"

	pb "github.com/brotherlogic/wantslist/proto"

	//Needed to pull in gzip encoding init
	_ "google.golang.org/grpc/encoding/gzip"
)

func main() {
	ctx, cancel := utils.BuildContext("wantslist-cli", "wantslist")
	defer cancel()

	conn, err := utils.LFDialServer(ctx, "wantslist")
	if err != nil {
		log.Fatalf("Unable to dial: %v", err)
	}
	defer conn.Close()

	client := pb.NewWantServiceClient(conn)

	switch os.Args[1] {
	case "get":
		lists, err := client.GetWantList(ctx, &pb.GetWantListRequest{})
		if err != nil {
			log.Fatalf("Error getting wantlists: %v", err)
		}

		fmt.Printf("Found %v lists\n", len(lists.Lists))
		for i, list := range lists.Lists {
			fmt.Printf("List %v. %v [%v]\n", (i + 1), list.Name, list.GetType())
			for _, entry := range list.Wants {
				fmt.Printf("  %v. %v (%v)\n", entry.Index, entry.Status, entry.Want)
			}
		}
	case "delete":
		_, err := client.DeleteWantList(ctx, &pb.DeleteWantlistRequest{Name: os.Args[2]})
		if err != nil {
			log.Fatalf("Error getting wantlists: %v", err)
		}
	case "add":
		bits := strings.Split(os.Args[2], ":")
		name := os.Args[2]
		speed := pb.WantList_STANDARD
		if len(bits) > 1 {
			name = bits[1]
			speed = pb.WantList_RAPID
		}
		list := &pb.WantList{Name: name, Type: speed, Wants: []*pb.WantListEntry{}}
		for i, v := range os.Args[3:] {
			val, err := strconv.Atoi(v)
			if err != nil {
				log.Fatalf("Cannot parse %v", v)
			}
			list.Wants = append(list.Wants, &pb.WantListEntry{Index: int32(i + 1), Want: int32(val)})
		}

		_, err := client.AddWantList(ctx, &pb.AddWantListRequest{Add: list})
		if err != nil {
			log.Fatalf("Error adding wantlist: %v", err)
		}
	case "file":
		list := &pb.WantList{Wants: []*pb.WantListEntry{}}

		file, err := os.Open(os.Args[2])
		if err != nil {
			log.Fatalf("Error reading file: %v", err)
		}
		defer file.Close()

		scanner := bufio.NewScanner(file)
		scanner.Scan()
		bits := strings.Split(scanner.Text(), ":")
		name := scanner.Text()
		speed := pb.WantList_STANDARD
		if len(bits) > 1 {
			name = bits[1]
			speed = pb.WantList_RAPID
		}

		list.Name = name
		list.Type = speed

		index := int32(1)
		for scanner.Scan() {
			iv, err := strconv.Atoi(scanner.Text())
			if err != nil {
				log.Fatalf("Parse error: %v", err)
			}
			list.Wants = append(list.Wants, &pb.WantListEntry{Index: index, Want: int32(iv)})
			index++
		}

		resp, err := client.AddWantList(ctx, &pb.AddWantListRequest{Add: list})
		if err != nil {
			log.Fatalf("Error adding wantlist: %v", err)
		}
		log.Printf("ADDED: %v", resp)
	}
}

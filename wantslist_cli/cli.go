package main

import (
	"bufio"
	"fmt"
	"log"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/brotherlogic/goserver/utils"

	pbrc "github.com/brotherlogic/recordcollection/proto"
	pb "github.com/brotherlogic/wantslist/proto"
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
	case "ping":
		num, err := strconv.ParseInt(os.Args[2], 10, 32)
		if err != nil {
			log.Fatalf("Bad num: %v", err)
		}

		client2 := pbrc.NewClientUpdateServiceClient(conn)
		_, err = client2.ClientUpdate(ctx, &pbrc.ClientUpdateRequest{InstanceId: int32(num)})
		if err != nil {
			log.Fatalf("Bad call: %v", err)
		}

	case "get":
		var lists *pb.GetWantListResponse
		var err error
		if len(os.Args) > 1 {
			lists, err = client.GetWantList(ctx, &pb.GetWantListRequest{Name: os.Args[2]})
		} else {
			lists, err = client.GetWantList(ctx, &pb.GetWantListRequest{})
		}
		if err != nil {
			log.Fatalf("Error getting wantlists: %v", err)
		}

		fmt.Printf("Found %v lists\n", len(lists.Lists))
		for i, list := range lists.Lists {
			fmt.Printf("List %v. %v [%v ; %v] %v\n", (i + 1), list.Name, list.GetType(), list.GetBudget(), list.GetOverallEstimatedCost())
			for _, entry := range list.Wants {
				fmt.Printf("  %v. %v (%v [%v] )\n", entry.Index, entry.Status, entry.Want, entry.GetEstimatedCost())
			}
		}
	case "delete":
		num, err := strconv.ParseInt(os.Args[3], 10, 32)
		if err != nil {
			log.Fatalf("Bad num: %v", err)
		}
		val, err := client.DeleteWantListItem(ctx, &pb.DeleteWantListItemRequest{ListName: os.Args[2], Entry: &pb.WantListEntry{Want: int32(num)}})
		log.Printf("%v -> %v", val, err)
	case "sdelete":
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
		name := bits[0]

		switch bits[1] {
		case "STANDARD":
			list.Type = pb.WantList_STANDARD
		case "ALL_IN":
			list.Type = pb.WantList_ALL_IN
		case "RAPID":
			list.Type = pb.WantList_RAPID
		default:
			log.Fatalf("%v is an unknown speed", bits[1])
		}

		switch bits[2] {
		case "year":
			list.RetireTime = time.Date(time.Now().Year()+1, time.Month(1), 1, 0, 0, 0, 0, time.Local).Unix()
		default:
			log.Fatalf("%v is an unknown runtime", bits[2])
		}

		list.Budget = bits[3]
		list.Name = name

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

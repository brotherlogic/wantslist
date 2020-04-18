protoc --proto_path ../../../ -I=./proto --go_out=plugins=grpc:./proto proto/wantslist.proto
mv proto/github.com/brotherlogic/wantslist/proto ./proto

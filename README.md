# grpcGen

A tool for auto converting go code into [gRPC](http://www.grpc.io/) [Protobuf](https://developers.google.com/protocol-buffers/) file.

## Howto
simply type these in a go file:
```go
package YourPackageName
//go:generate grpcGen $GOFILE
```
and then in terminal:
```shell
$ go generate
```
it will generated an example go code for you to modify.
once you done the modification, then just type the same command as previous one in the terminal, it will generating an protobuf file and calling gRPC tool to generate an gRPC go code for you. **THAT'S IT**

## More Detail
The working diagram will be like this:

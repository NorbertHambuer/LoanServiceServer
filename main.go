package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"github.com/jinzhu/gorm"
	_ "github.com/jinzhu/gorm/dialects/mysql"
	"github.com/norby7/LoanServiceServer/proto"
	"google.golang.org/grpc"
	"log"
	"net"
	"strconv"
	"time"
)

var conn *gorm.DB

var (
	tls        = flag.Bool("tls", false, "Connection uses TLS if true, else plain TCP")
	certFile   = flag.String("cert_file", "", "The TLS cert file")
	keyFile    = flag.String("key_file", "", "The TLS key file")
	jsonDBFile = flag.String("json_db_file", "", "A json file containing a list of features")
	port       = flag.Int("port", 10000, "The server port")
)

type User struct {
	gorm.Model
	Name     string
	Password string
}

type Loan struct {
	gorm.Model
	Amount int
	UserId int
	PayDay time.Time
}

type LoanServer struct {
}

func (s *LoanServer) LoginClient(ctx context.Context, userCredentials *proto.UserCredentials) (*proto.Client, error) {
	var user User
	var client proto.Client
	conn.First(&user, "name = ?", userCredentials.Name)

	if user.Password == userCredentials.Password {
		var loan Loan
		conn.First(&loan, "userid = ?", user.ID)

		client.Name = user.Name
		client.Amount = int32(loan.Amount)
		client.Id = int32(user.ID)
		client.PayDay = loan.PayDay.Unix()
		return &client, nil
	}

	return &client, errors.New("invalid username/password")
}

func (s *LoanServer) RegisterClient(ctx context.Context, userRegisterCredentials *proto.UserRegisterCredentials) (*proto.Client, error) {
	var user User
	var client proto.Client
	if conn.First(&user, "name = ?", userRegisterCredentials.Name).RecordNotFound() {
		conn.Create(&User{
			Name:     userRegisterCredentials.Name,
			Password: userRegisterCredentials.Password,
		}).Scan(&user)

		client.Name = user.Name
		client.Id = int32(user.ID)

		return &client, nil
	}

	return &client, errors.New("username already exists")
}

func (s *LoanServer) RequestAmount(ctx context.Context, loanRequest *proto.LoanRequest) (*proto.LoanInfo, error) {
	var loan Loan
	if conn.First(&loan, "user_id = ?", loanRequest.ClientId).RecordNotFound() {
		conn.Create(&Loan{
			Amount: int(loanRequest.Amount),
			UserId: int(loanRequest.ClientId),
			PayDay: time.Now().AddDate(1, 0, 0),
		}).Scan(&loan)

		return &proto.LoanInfo{
			Id:     int32(loan.ID),
			Amount: loanRequest.Amount,
			PayDay: time.Now().AddDate(1, 0, 0).Unix(),
		}, nil
	}

	return &proto.LoanInfo{}, errors.New("user already has an active loan")
}

func (s *LoanServer) CheckClientStatus(ctx context.Context, client *proto.Client) (*proto.LoanInfo, error) {
	var loan Loan

	conn.First(&loan, "user_id = ?", client.Id)

	return &proto.LoanInfo{
		Id:     int32(loan.ID),
		Amount: int32(loan.Amount),
		PayDay: loan.PayDay.Unix(),
	}, nil
}

func (s *LoanServer) PayLoan(ctx context.Context, client *proto.Client) (*proto.OperationMsg, error) {
	var loan Loan
	if !conn.First(&loan, "user_id = ?", client.Id).RecordNotFound() {
		conn.Delete(&loan)

		return &proto.OperationMsg{
			Msg: "Loan payed",
		}, nil
	}

	return &proto.OperationMsg{}, errors.New("no loan registered for current user")
}

func InitiateDatabaseConnection() *gorm.DB {
	connection, err := gorm.Open("mysql", "root:@(localhost)/loan_service?charset=utf8&parseTime=True&loc=Local")
	if err != nil {
		panic("Connection to database failed")
	}

	return connection
}

func main() {
	conn = InitiateDatabaseConnection()

	conn.AutoMigrate(&User{})
	conn.AutoMigrate(&Loan{})

	fmt.Println("Migrations complete")

	flag.Parse()

	fmt.Println("Parsed parameters")
	lis, err := net.Listen("tcp", fmt.Sprintf(":%d", *port))
	if err != nil {
		log.Fatalf("failed to listen: %v", err)
	}

	fmt.Println("Listening to TCP")

	grpcServer := grpc.NewServer()

	fmt.Println("Created new server")
	proto.RegisterLoanServiceServer(grpcServer, &LoanServer{})

	fmt.Println("Registered service interface")

	err = grpcServer.Serve(lis)

	fmt.Println("Served GRPC server")
	if err != nil {
		panic(err)
	}

	fmt.Println("Service started on port " + strconv.Itoa(*port))
}

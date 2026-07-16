package bridge

import (
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"time"

	"github.com/leandrofars/oktopus/internal/entity"
	local "github.com/leandrofars/oktopus/internal/nats"
	"github.com/leandrofars/oktopus/internal/utils"
	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
)

var errNatsMsgReceivedWithErrorData = errors.New("Nats message received with error data")
var errNatsRequestTimeout = errors.New("Nats message response timeout")
var errInvalidUSPRequest = errors.New("invalid USP request record")

type Bridge struct {
	js jetstream.JetStream
	nc *nats.Conn
}

func NewBridge(js jetstream.JetStream, nc *nats.Conn) Bridge {
	return Bridge{
		js: js,
		nc: nc,
	}
}

func NatsUspInteraction(
	subSubj, pubSubj string,
	body []byte,
	w http.ResponseWriter,
	nc *nats.Conn,
) ([]byte, error) {
	return natsUspInteraction(subSubj, pubSubj, body, w, nc, local.NATS_REQUEST_TIMEOUT)
}

func natsUspInteraction(
	subSubj, pubSubj string,
	body []byte,
	w http.ResponseWriter,
	nc *nats.Conn,
	timeout time.Duration,
) ([]byte, error) {
	log.Println("Sending usp message")
	log.Println("subSubj: ", subSubj)
	log.Println("pubSubj: ", pubSubj)

	expectedMsgID, err := extractUSPMsgIDFromRecord(body)
	if err != nil {
		log.Printf("NatsUspInteraction: invalid request record: %v", err)
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write(utils.Marshall("Invalid USP request record: " + err.Error()))
		return nil, errors.Join(errInvalidUSPRequest, err)
	}

	ch := make(chan *nats.Msg, 64)
	sub, err := nc.ChanSubscribe(subSubj, ch)
	if err != nil {
		log.Println(err)
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write(utils.Marshall("Error to communicate with nats: " + err.Error()))
		return nil, err
	}
	defer func() {
		if err := sub.Unsubscribe(); err != nil {
			log.Printf("Error unsubscribing from %s: %v", subSubj, err)
		}
	}()

	// Ensure the response subscription is active on the server before publishing.
	if err := nc.Flush(); err != nil {
		log.Println(err)
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write(utils.Marshall("Error to communicate with nats: " + err.Error()))
		return nil, err
	}
	if err := nc.Publish(pubSubj, body); err != nil {
		log.Println(err)
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write(utils.Marshall("Error to communicate with nats: " + err.Error()))
		return nil, err
	}

	timer := time.NewTimer(timeout)
	defer timer.Stop()

	for {
		select {
		case msg := <-ch:
			if msg == nil {
				continue
			}
			respMsgID, err := extractUSPMsgIDFromRecord(msg.Data)
			if err != nil || respMsgID != expectedMsgID {
				log.Printf("NatsUspInteraction: discarding response (want msg_id=%s, got=%s, err=%v)",
					expectedMsgID, respMsgID, err)
				continue
			}
			log.Println("Received an usp message response")
			return msg.Data, nil
		case <-timer.C:
			log.Println("usp message response timeout")
			w.WriteHeader(http.StatusGatewayTimeout)
			_, _ = w.Write(utils.Marshall("usp message response timeout"))
			return nil, errNatsRequestTimeout
		}
	}
}

func NatsCustomReq[T entity.DataType](
	subSubj, pubSubj string,
	body []byte,
	w http.ResponseWriter,
	nc *nats.Conn,
) (interface{}, error) {
	return natsCustomReq[T](subSubj, pubSubj, body, w, nc, local.NATS_REQUEST_TIMEOUT)
}

func natsCustomReq[T entity.DataType](
	subSubj, pubSubj string,
	body []byte,
	w http.ResponseWriter,
	nc *nats.Conn,
	timeout time.Duration,
) (interface{}, error) {
	var answer T

	ch := make(chan *nats.Msg, 64)
	sub, err := nc.ChanSubscribe(subSubj, ch)
	if err != nil {
		log.Println(err)
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write(utils.Marshall("Error to communicate with nats: " + err.Error()))
		return nil, err
	}
	defer func() {
		if err := sub.Unsubscribe(); err != nil {
			log.Printf("Error unsubscribing from %s: %v", subSubj, err)
		}
	}()

	if err := nc.Flush(); err != nil {
		log.Println(err)
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write(utils.Marshall("Error to communicate with nats: " + err.Error()))
		return nil, err
	}
	if err := nc.Publish(pubSubj, body); err != nil {
		log.Println(err)
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write(utils.Marshall("Error to communicate with nats: " + err.Error()))
		return nil, err
	}

	timer := time.NewTimer(timeout)
	defer timer.Stop()
	select {
	case msg := <-ch:
		if msg == nil {
			log.Println("NATS response subscription closed")
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = w.Write(utils.Marshall("NATS response subscription closed"))
			return nil, nats.ErrConnectionClosed
		}
		log.Println("Received an api message response")
		if err := json.Unmarshal(msg.Data, &answer); err != nil {
			log.Println(err)
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = w.Write(msg.Data)
			return nil, err
		}
		return answer, nil
	case <-timer.C:
		log.Println("Api message response timeout")
		w.WriteHeader(http.StatusGatewayTimeout)
		_, _ = w.Write(utils.Marshall("api message response timeout"))
		return nil, errNatsRequestTimeout
	}
}

/*
- makes a request to nats topic

- handle nats communication

- verify if received data is of error type
*/
func NatsReq[T entity.DataType](
	subj string,
	body []byte,
	w http.ResponseWriter,
	nc *nats.Conn,
) (*entity.MsgAnswer[T], error) {

	var answer *entity.MsgAnswer[T]

	msg, err := nc.Request(subj, body, local.NATS_REQUEST_TIMEOUT)
	if err != nil {
		log.Println(err)
		w.WriteHeader(http.StatusInternalServerError)
		w.Write(utils.Marshall("Error to communicate with nats: " + err.Error()))
		return nil, err
	}

	err = json.Unmarshal(msg.Data, &answer)
	if err != nil {
		var errMsg *entity.MsgAnswer[*string]
		err = json.Unmarshal(msg.Data, &errMsg)

		if err != nil {
			log.Println("Bad answer message formatting: ", err.Error())
			w.WriteHeader(http.StatusInternalServerError)
			w.Write(msg.Data)
			return nil, err
		}

		log.Printf("message received, msg: %s, code: %d", *errMsg.Msg, errMsg.Code)
		w.WriteHeader(errMsg.Code)
		w.Write(utils.Marshall(*errMsg.Msg))
		return nil, errNatsMsgReceivedWithErrorData
	}

	return answer, nil
}

func NatsReqWithoutHttpSet[T entity.DataType](
	subj string,
	body []byte,
	nc *nats.Conn,
) (*entity.MsgAnswer[T], error) {

	var answer *entity.MsgAnswer[T]

	msg, err := nc.Request(subj, body, local.NATS_REQUEST_TIMEOUT)
	if err != nil {
		log.Println(err)
		return nil, err
	}

	err = json.Unmarshal(msg.Data, &answer)
	if err != nil {

		var errMsg *entity.MsgAnswer[*string]
		err = json.Unmarshal(msg.Data, &errMsg)

		if err != nil {
			log.Println("Bad answer message formatting: ", err.Error())
			return nil, err
		}

		log.Printf("Error message received, msg: %s, code: %d", *errMsg.Msg, errMsg.Code)
		return nil, errNatsMsgReceivedWithErrorData
	}

	return answer, nil
}

func NatsCwmpInteraction(
	subj string,
	body []byte,
	w http.ResponseWriter,
	nc *nats.Conn,
) ([]byte, error) {

	log.Println("Sending cwmp message")
	log.Println("Subject: ", subj)

	var answer entity.MsgAnswer[[]byte]

	msg, err := nc.Request(subj, body, local.NATS_REQUEST_TIMEOUT)
	if err != nil {
		log.Println(err)
		w.WriteHeader(http.StatusInternalServerError)
		w.Write(utils.Marshall("Error to communicate with nats: " + err.Error()))
		return nil, err
	}

	err = json.Unmarshal(msg.Data, &answer)
	if err != nil {

		var errMsg *entity.MsgAnswer[*string]
		err = json.Unmarshal(msg.Data, &errMsg)

		if err != nil {
			log.Println("Bad answer message formatting: ", err.Error())
			w.WriteHeader(http.StatusInternalServerError)
			w.Write(msg.Data)
			return nil, err
		}

		log.Printf("Error message received, msg: %s, code: %d", *errMsg.Msg, errMsg.Code)
		w.WriteHeader(errMsg.Code)
		w.Write(utils.Marshall(*errMsg.Msg))
		return nil, errNatsMsgReceivedWithErrorData
	}

	return answer.Msg, nil
}

func NatsEnterpriseInteraction(
	subj string,
	body []byte,
	w http.ResponseWriter,
	nc *nats.Conn,
) error {

	log.Println("Sending enterprise message")
	log.Println("Subject: ", subj)

	var answer entity.MsgAnswer[[]byte]

	msg, err := nc.Request(subj, body, local.NATS_REQUEST_TIMEOUT+20*time.Second)
	if err != nil {
		if err == nats.ErrNoResponders {
			w.WriteHeader(http.StatusInternalServerError)
			w.Write(utils.Marshall("You have no enterprise license, to get one contact: sales@oktopus.app.br"))
			return err
		}
		w.WriteHeader(http.StatusInternalServerError)
		w.Write(utils.Marshall("Error to communicate with nats:" + err.Error()))
		return err
	}

	err = json.Unmarshal(msg.Data, &answer)
	if err != nil {

		var errMsg *entity.MsgAnswer[*string]
		err = json.Unmarshal(msg.Data, &errMsg)

		if err != nil {
			log.Println("Bad answer message formatting: ", err.Error())
			w.WriteHeader(http.StatusInternalServerError)
			w.Write(msg.Data)
			return err
		}

		log.Printf("Error message received, msg: %s, code: %d", *errMsg.Msg, errMsg.Code)
		w.WriteHeader(errMsg.Code)
		w.Write(utils.Marshall(*errMsg.Msg))
		return errNatsMsgReceivedWithErrorData
	}

	w.Write(answer.Msg)
	return nil
}

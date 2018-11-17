package main

import (
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"os/signal"
	"time"

	bbhw "github.com/btittelbach/go-bbhw"
	"github.com/streadway/amqp"
)

/*
//Mapping between Px_y notation and port numbers can be found here:
// https://github.com/adafruit/adafruit-beaglebone-io-python/blob/master/source/common.c
// P9_22 is 2
// P8_8 is 67
// etc.

*/

const (
	exchangeCtrl   = "gpiobutton_ctrl"
	exchangeEvents = "gpiobutton_events"
)

type GPIOButtonConfig struct {
	I2CAddress byte
	I2CLane    int
	GpioPin    uint
	RmqServer  string
	ButtonName string
}

type GPIOButtonMQ struct {
	config    GPIOButtonConfig
	ctrl      *bbhw.MMappedGPIO
	killed    bool
	conn      *amqp.Connection
	ch        *amqp.Channel
	ctrlQueue amqp.Queue
}

func InitGPIOButtonMQ(configFile string) (*GPIOButtonMQ, error) {
	var bmq GPIOButtonMQ

	//Load config
	data, err := ioutil.ReadFile(configFile)
	if err != nil {
		return nil, err
	}
	err = json.Unmarshal(data, &bmq.config)
	if err != nil {
		return nil, err
	}

	bmq.ctrl = bbhw.NewMMappedGPIO(bmq.config.GpioPin, bbhw.IN)

	//Setup AMQP
	bmq.conn, err = amqp.Dial(bmq.config.RmqServer)
	if err != nil {
		return nil, err
	}

	bmq.ch, err = bmq.conn.Channel()
	if err != nil {
		return nil, err
	}

	//Setup Control exchange & queue
	err = bmq.ch.ExchangeDeclare(
		exchangeCtrl, // name
		"fanout",     // type
		true,         // durable
		false,        // auto-deleted
		false,        // internal
		false,        // no-wait
		nil,          // arguments
	)
	if err != nil {
		return nil, err
	}

	bmq.ctrlQueue, err = bmq.ch.QueueDeclare(
		"",    // name
		false, // durable
		false, // delete when usused
		true,  // exclusive
		false, // no-wait
		nil,   // arguments
	)
	if err != nil {
		return nil, err
	}

	//Bind this queue to this exchange so that exchange will publish here
	err = bmq.ch.QueueBind(
		bmq.ctrlQueue.Name, // queue name
		"",                 // routing key
		exchangeCtrl,       // exchange
		false,
		nil)

	//Setup events exchange
	err = bmq.ch.ExchangeDeclare(
		exchangeEvents, // name
		"fanout",       // type
		true,           // durable
		false,          // auto-deleted
		false,          // internal
		false,          // no-wait
		nil,            // arguments
	)
	if err != nil {
		return nil, err
	}

	bmq.killed = false
	return &bmq, nil
}

func (bmq *GPIOButtonMQ) Destroy() {

}

func (bmq *GPIOButtonMQ) ReceiveCommands() error {
	msgs, err := bmq.ch.Consume(
		bmq.ctrlQueue.Name, // queue
		"",                 // consumer
		true,               // auto-ack
		false,              // exclusive
		false,              // no-local
		false,              // no-wait
		nil,                // args
	)
	if err != nil {
		return err
	}

	go func() {
		for _ = range msgs {

		}
	}()

	return nil
}

func (bmq *GPIOButtonMQ) EmitEvents() error {

	pressed := false
	var pressStart time.Time

	for {
		if bmq.killed == true {
			break
		}

		time.Sleep(100 * time.Millisecond)
		state, err := bmq.ctrl.GetState()
		if err != nil {
			continue
		}

		if (state == true) && (pressed == false) {
			pressed = true
			pressStart = time.Now()
		}

		if (state == false) && (pressed == true) {
			pressed = false
			pressDuration := time.Since(pressStart)

			buf := make([]byte, 8)
			binary.BigEndian.PutUint64(buf, uint64(pressDuration.Nanoseconds()/1000000))

			err := bmq.ch.Publish(
				exchangeEvents, // exchange
				"",             // routing key
				false,          // mandatory
				false,          // immediate
				amqp.Publishing{
					ContentType: fmt.Sprintf("application/button_press_%s", bmq.config.ButtonName),
					Body:        buf,
				})
			if err != nil {
				continue
			}
			log.Printf("Sent button press %s (%d ms)", fmt.Sprintf("application/button_press_%s", bmq.config.ButtonName), uint64(pressDuration.Nanoseconds()/1000000))
		}

	}

	return nil
}

func main() {

	if len(os.Args) != 2 {
		fmt.Println("Usage: " + os.Args[0] + " <config file>")
		return
	}
	bmq, err := InitGPIOButtonMQ(os.Args[1])
	if err != nil {
		log.Fatalf("Failed to init GPIOButtonMQ: %s", err)
		return
	}
	defer bmq.Destroy()

	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt)
	go func() {
		for sig := range c {
			fmt.Println(sig)
			bmq.killed = true
		}
	}()

	bmq.ReceiveCommands()
	bmq.EmitEvents()

	bmq.Destroy()
}

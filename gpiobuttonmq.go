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

const (
	i2cAddress = 0x4B
	i2cLane    = 2
)

type Controls struct {
	button *bbhw.MMappedGPIO
}

func New() *Controls {
	var ctrl Controls
	ctrl.button = bbhw.NewMMappedGPIO(2, bbhw.IN) // Right grove port is P9_22

	return &ctrl
}

func (ctrl *Controls) Destroy() {
}

func (ctrl *Controls) GetPressed() (bool, error) {
	st, err := ctrl.button.GetState()
	if err != nil {
		return false, err
	}

	return st, nil
}
*/

type GPIOButtonConfig struct {
	I2CAddress byte
	I2CLane    int
	GpioPin    uint
	RmqServer  string
}

type GPIOButtonMQ struct {
	config      GPIOButtonConfig
	ctrl        *bbhw.MMappedGPIO
	killed      bool
	conn        *amqp.Connection
	ch          *amqp.Channel
	ctrlQueue   amqp.Queue
	streamQueue amqp.Queue
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

	//Create control queue
	bmq.ctrlQueue, err = bmq.ch.QueueDeclare(
		"gpiobutton_ctrl", // name
		false,             // durable
		false,             // delete when unused
		false,             // exclusive
		false,             // no-wait
		nil,               // arguments
	)
	if err != nil {
		return nil, err
	}

	//Create events queue
	bmq.streamQueue, err = bmq.ch.QueueDeclare(
		"gpiobutton_events", // name
		false,               // durable
		false,               // delete when unused
		false,               // exclusive
		true,                // no-wait
		nil,                 // arguments
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
				"",                   // exchange
				bmq.streamQueue.Name, // routing key
				false,                // mandatory
				false,                // immediate
				amqp.Publishing{
					ContentType: fmt.Sprintf("application/button_press"),
					Body:        buf,
				})
			if err != nil {
				continue
			}
			log.Printf("Sent button press (%d ms)", uint64(pressDuration.Nanoseconds()/1000000))
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

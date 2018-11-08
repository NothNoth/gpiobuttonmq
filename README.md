# GPIOButtonMQ

GPIO button service for rabbitmq


Simple service watching for button press on a GPIO pin (tested on a Beaglebone) and sending a message on a rabbitMQ exchange upon release (with press duration).

## Config

    {
      "I2CAddress": 75, <--- address of the I2C bus (0x4B)
      "I2CLane": 2,     <--- I2C Lane
      "GpioPin": 2,     <--- Pin where the button is connected
      "RmqServer": "amqp://guest:guest@localhost:5672/" <--- address of the AMQP server
    }


## AMQP

| AMQP exchange | IN/OUT | Content-Type | Data | Description |
| ------------ | ------ | ------------ | ---- | ----------- |
| gpiobutton_events | OUT | application/button_press | Timestamp in ms | Emitted when the button is released, data contains the pressure duration |
| gpiobutton_ctrl   | IN  | --         | --   | Unused      |


#!/usr/bin/env python

import eventlet
from flask import Flask, render_template
from flask_mqtt import Mqtt
from flask_socketio import SocketIO

eventlet.monkey_patch()

app = Flask(__name__)
app.config['MQTT_BROKER_URL'] = '10.140.80.1'
app.config['MQTT_BROKER_PORT'] = 1883
app.config['MQTT_REFRESH_TIME'] = 1.0

mqtt = Mqtt(app)
socketio = SocketIO(app)

@mqtt.on_connect()
def handle_connect(client, userdata, flags, rc):
    mqtt.subscribe('net-lama/log')
    #mqtt.subscribe('sensorpi/comm')

@mqtt.on_message()
def handle_mqtt_message(client, userdata, message):
    print('DATA')
    data = {'topic': message.topic, 'payload': message.payload.decode()}
    socketio.emit('mqtt_message', data=data)
    print('mqttData: ' + data['payload'])

@app.route('/')
def index():
    return render_template('graph.html')

@mqtt.on_log()
def handle_logging(client, userdata, level, buf):
    print(level, buf)

socketio.run(app, host='0.0.0.0', port=5500, use_reloader=True, debug=True)
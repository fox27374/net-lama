#!/usr/bin/env python

from flask_marshmallow import Schema, fields
from models.config import MqttModel, HecForwarderModel, NetworkTestModel

class MQTTSchema(Schema):
    class Meta:
        model = MqttModel

    mqttServer = fields.Str()
    mqttPort = fields.Int()
    commandTopic = fields.Str()
    dataTopic = fields.Str()
    logTopic = fields.Str()


class HecForwarderSchema(Schema):
    class Meta:
        model = HecForwarderModel

    hecServer = fields.Str()
    hecPort = fields.Int()
    hecUrl = fields.Str()
    hecToken = fields.Str()


class NetworkTestSchema(Schema):
    class Meta:
        model = NetworkTestModel

    speedTestInterval = fields.Int()
    pingDestination = fields.List(fields.String())
    dnsQuery = fields.List(fields.String())
    dnsServer = fields.List(fields.String())


#class ClientSchema(Schema):
#    clientType = fields.Str()
#    clientId = fields.Str()

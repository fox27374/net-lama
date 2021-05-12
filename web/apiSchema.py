#!/usr/bin/env python

from marshmallow import Schema, fields

class MQTTSchema(Schema):
    mqttPort = fields.Str()
    mqttServer = fields.Str()
    commandTopic = fields.Str()
    dataTopic = fields.Str()
    logTopic = fields.Str()

class SensorPiSchema(Schema):
    Interface = fields.Str()
    Channels = fields.List(fields.String())
    Scantime = fields.Int()
    Channeltime = fields.Int()
    rssiThreshold = fields.Int()
    wlansFile = fields.Str()
    frameTypesFile = fields.Str()
    logFile = fields.Str()

class SplunkSchema(Schema):
    Server = fields.Str()
    Port = fields.Int()
    URL = fields.Str()
    Token = fields.Str()
    Bulk = fields.Int()

class WlanSensorSchema(Schema):
    scantime = fields.Int()
    interface = fields.String()
    channels = fields.List(fields.Int())
    scanTime = fields.Int()
    wlans = fields.Dict()

class ConfigSchema(Schema):
    SensorPi = fields.Nested(SensorPiSchema)
    MQTT = fields.Nested(MQTTSchema)
    Splunk = fields.Nested(SplunkSchema)
    WlanSensor = fields.Nested(WlanSensorSchema)

class ClientSchema(Schema):
    clientType = fields.Str()
    clientId = fields.Str()
    lastSeen = fields.Str()
    clientStatus = fields.Str()
    appStatus = fields.Str()
    commands = fields.List(fields.String())
    capabilities = fields.Dict()

class RegisterSchema(Schema):
    client = fields.Nested(ClientSchema)

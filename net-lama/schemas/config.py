from marshmallow import Schema, fields

class MqttSchema(Schema):
    mqttServer = fields.Str()
    mqttPort = fields.Int()
    commandTopic = fields.Str()
    dataTopic = fields.Str()
    logTopic = fields.Str()
    siteId = fields.Int()


class HecForwarderSchema(Schema):
    hecServer = fields.Str()
    hecPort = fields.Int()
    hecUrl = fields.Str()
    hecToken = fields.Str()


class NetworkTestSchema(Schema):
    speedTestInterval = fields.Int()
    pingDestination = fields.List(fields.Str)
    dnsQuery = fields.List(fields.Str)
    dnsServer = fields.List(fields.Str)

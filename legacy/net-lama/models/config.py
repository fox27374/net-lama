import site
from db.db import db
from json import loads, dumps

class MqttModel(db.Model):
    __tablename__ = 'mqttConfigs'
    configId = db.Column(db.Integer, primary_key=True)
    siteId = db.Column(db.Integer)
    mqttServer = db.Column(db.String(80))
    mqttPort = db.Column(db.Integer)
    commandTopic = db.Column(db.String(80))
    dataTopic = db.Column(db.String(80))
    logTopic = db.Column(db.String(80))

    # configId = db.Column(db.Integer, db.ForeignKey('configs.configId'))
    # siteId = db.Column(db.Integer, db.ForeignKey('sites.siteId'))
    # config = db.relationship('ConfigModel')
    # site = db.relationship('SiteModel')

    def __init__(self, mqttServer, mqttPort, commandTopic, dataTopic, logTopic, siteId=None):
        self.mqttServer = mqttServer
        self.mqttPort = mqttPort
        self.commandTopic = commandTopic
        self.dataTopic = dataTopic
        self.logTopic = logTopic
        self.siteId = siteId if siteId else 1

    def json(self):
        return {
            "configId": self.configId,
            "mqttServer": self.mqttServer,
            "mqttPort": self.mqttPort,
            "commandTopic": self.commandTopic,
            "dataTopic": self.dataTopic,
            "logTopic": self.logTopic,
            "siteId": self.siteId
            }

    @classmethod
    def findByConfigId(cls, configId):
        return cls.query.filter_by(configId=configId).first()

    @classmethod
    def findBySiteId(cls, siteId):
        return cls.query.filter_by(siteId=siteId).first()

    def save(self):
        db.session.add(self)
        db.session.commit()

    def delete(self):
        db.session.delete(self)
        db.session.commit()


class HecForwarderModel(db.Model):
    __tablename__ = 'hecForwarderConfigs'
    configId = db.Column(db.Integer, primary_key=True)
    siteId = db.Column(db.Integer)
    hecServer = db.Column(db.String(80))
    hecPort = db.Column(db.Integer)
    hecUrl = db.Column(db.String(80))
    hecToken = db.Column(db.String(80))

    def __init__(self, hecServer, hecPort, hecUrl, hecToken, siteId=None):
        self.hecServer = hecServer
        self.hecPort = hecPort
        self.hecUrl = hecUrl
        self.hecToken = hecToken
        self.siteId = siteId if siteId else 1

    def json(self):
        return {
            "configId": self.configId,
            "hecServer": self.hecServer,
            "hecPort": self.hecPort,
            "hecUrl": self.hecUrl,
            "hecToken": self.hecToken,
            "siteId": self.siteId
            }

    @classmethod
    def findByConfigId(cls, configId):
        return cls.query.filter_by(configId=configId).first()

    @classmethod
    def findBySiteId(cls, siteId):
        return cls.query.filter_by(siteId=siteId).first()

    def save(self):
        db.session.add(self)
        db.session.commit()

    def delete(self):
        db.session.delete(self)
        db.session.commit()


class NetworkTestModel(db.Model):
    __tablename__ = 'networkTestConfigs'
    siteId = db.Column(db.Integer)
    configId = db.Column(db.Integer, primary_key=True)
    speedTestInterval = db.Column(db.Integer)
    pingDestination = db.Column(db.Text)
    dnsQuery = db.Column(db.Text)
    dnsServer = db.Column(db.Text)

    def __init__(self, speedTestInterval, pingDestination, dnsQuery, dnsServer, siteId=None):
        self.speedTestInterval = speedTestInterval
        self.pingDestination = dumps(pingDestination)
        self.dnsQuery = dumps(dnsQuery)
        self.dnsServer = dumps(dnsServer)
        self.siteId = siteId if siteId else 1

    def json(self):
        return {
            "configId": self.configId,
            "speedTestInterval": self.speedTestInterval,
            "pingDestination": loads(self.pingDestination),
            "dnsQuery": loads(self.dnsQuery),
            "dnsServer": loads(self.dnsServer),
            "siteId": self.siteId
            }

    @classmethod
    def findByConfigId(cls, configId):
        return cls.query.filter_by(configId=configId).first()

    @classmethod
    def findBySiteId(cls, siteId):
        return cls.query.filter_by(siteId=siteId).first()

    def save(self):
        db.session.add(self)
        db.session.commit()

    def delete(self):
        db.session.delete(self)
        db.session.commit()
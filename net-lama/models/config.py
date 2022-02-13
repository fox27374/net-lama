from db.db import db
from json import loads

class MqttModel(db.Model):
    __tablename__ = 'mqttConfigs'
    configId = db.Column(db.Integer, primary_key=True)
    mqttServer = db.Column(db.String(80))
    mqttPort = db.Column(db.Integer)
    commandTopic = db.Column(db.String(80))
    dataTopic = db.Column(db.String(80))
    logTopic = db.Column(db.String(80))

    # configId = db.Column(db.Integer, db.ForeignKey('configs.configId'))
    # siteId = db.Column(db.Integer, db.ForeignKey('sites.siteId'))
    # config = db.relationship('ConfigModel')
    # site = db.relationship('SiteModel')

    def __init__(self, mqttServer, mqttPort, commandTopic, dataTopic, logTopic):
        self.mqttServer = mqttServer
        self.mqttPort = mqttPort
        self.commandTopic = commandTopic
        self.dataTopic = dataTopic
        self.logTopic = logTopic

    def json(self):
        return {
            "configId": self.configId,
            "mqttServer": self.mqttServer,
            "mqttPort": self.mqttPort,
            "commandTopic": self.commandTopic,
            "dataTopic": self.dataTopic,
            "logTopic": self.logTopic
            }

    @classmethod
    def find(cls, configId):
        return cls.query.filter_by(configId=configId).first()

    def save(self):
        db.session.add(self)
        db.session.commit()

    def delete(self):
        db.session.delete(self)
        db.session.commit()
    

class HecForwarderModel(db.Model):
    __tablename__ = 'hecForwarderConfigs'
    configId = db.Column(db.Integer, primary_key=True)
    hecServer = db.Column(db.String(80))
    hecPort = db.Column(db.Integer)
    hecUrl = db.Column(db.String(80))
    hecToken = db.Column(db.String(80))

    def __init__(self, hecServer, hecPort, hecUrl, hecToken):
        self.hecServer = hecServer
        self.hecPort = hecPort
        self.hecUrl = hecUrl
        self.hecToken = hecToken

    def json(self):
        return {
            "configId": self.configId,
            "hecServer": self.hecServer,
            "hecPort": self.hecPort,
            "hecUrl": self.hecUrl,
            "hecToken": self.hecToken
            }

    @classmethod
    def find(cls, configId):
        return cls.query.filter_by(configId=configId).first()

    def save(self):
        db.session.add(self)
        db.session.commit()

    def delete(self):
        db.session.delete(self)
        db.session.commit()


class NetworkTestModel(db.Model):
    __tablename__ = 'networkTestConfigs'
    configId = db.Column(db.Integer, primary_key=True)
    speedTestInterval = db.Column(db.Integer)
    pingDestination = db.Column(db.Text)
    dnsQuery = db.Column(db.Text)
    dnsServer = db.Column(db.Text)

    def __init__(self, speedTestInterval, pingDestination, dnsQuery, dnsServer):
        self.speedTestInterval = speedTestInterval
        self.pingDestination = loads(pingDestination)
        self.dnsQuery = loads(dnsQuery)
        self.dnsServer = loads(dnsServer)

    def json(self):
        return {
            "configId": self.configId,
            "speedTestInterval": self.speedTestInterval,
            "pingDestination": self.pingDestination,
            "dnsQuery": self.dnsQuery,
            "dnsServer": self.dnsServer
            }

    @classmethod
    def find(cls, configId):
        return cls.query.filter_by(configId=configId).first()

    def save(self):
        db.session.add(self)
        db.session.commit()

    def delete(self):
        db.session.delete(self)
        db.session.commit()
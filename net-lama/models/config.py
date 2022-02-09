from db.db import db

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
    
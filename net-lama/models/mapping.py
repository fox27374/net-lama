from db.db import db

class MappingModel(db.Model):
    __tablename__ = 'orgmappings'
    id = db.Column(db.Integer, primary_key=True)
    orgId = db.Column(db.Integer)
    siteId = db.Column(db.Integer)
    configId = db.Column(db.Integer)
    clientId = db.Column(db.String(8))

    def __init__(self, orgId, siteId, configId, clientId):
        self.orgId = orgId
        self.siteId = siteId
        self.configId = configId
        self.clientId = clientId

    @classmethod
    def findByClientId(cls, clientId):
        return cls.query.filter_by(clientId=clientId).first()

    @classmethod
    def findByConfigId(cls, configId):
        return cls.query.filter_by(configId=configId).first()

    @classmethod
    def findBySiteId(cls, siteId):
        return cls.query.filter_by(siteId=siteId)

    @classmethod
    def findByOrgId(cls, orgId):
        return cls.query.filter_by(orgId=orgId)

    def save(self):
        db.session.add(self)
        db.session.commit()

    def delete(self):
        db.session.delete(self)
        db.session.commit()
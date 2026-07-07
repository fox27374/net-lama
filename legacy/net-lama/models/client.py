from db.db import db
from flask_marshmallow import Marshmallow

class ClientModel(db.Model):
    __tablename__ = 'clients'
    _id = db.Column(db.Integer, primary_key=True)
    clientId = db.Column(db.String(8))
    siteId = db.Column(db.Integer)
    clientType = db.Column(db.String(80))

    def __init__(self, clientId, clientType, siteId=None):
        self.clientId = clientId
        self.clientType = clientType
        self.siteId = siteId if siteId else 1

    def json(self):
        return {
            "clientId": self.clientId,
            "clientType": self.clientType,
            "siteId": self.siteId
            }

    @classmethod
    def findById(cls, clientId):
        return cls.query.filter_by(clientId=clientId).first()

    def save(self):
        db.session.add(self)
        db.session.commit()

    def delete(self):
        db.session.delete(self)
        db.session.commit()
    
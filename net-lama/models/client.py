from db.db import db

class ClientModel(db.Model):
    __tablename__ = 'clients'
    _id = db.Column(db.Integer, primary_key=True)
    clientId = db.Column(db.String(8))
    clientType = db.Column(db.String(80))

    def __init__(self, clientId, clientType):
        self.clientId = clientId
        self.clientType = clientType

    def json(self):
        return {
            "clientId": self.clientId,
            "clientType": self.clientType,
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
    
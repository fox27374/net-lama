from db.db import db

class OrganizationModel(db.Model):
    __tablename__ = 'organizations'
    orgId = db.Column(db.Integer, primary_key=True)
    orgName = db.Column(db.String(80))

    def __init__(self, orgName):
        self.orgName = orgName

    def json(self):
        return {
            "orgId": self.orgId,
            "orgName": self.orgName
            }

    @classmethod
    def findById(cls, orgId):
        return cls.query.filter_by(orgId=orgId).first()

    @classmethod
    def findByName(cls, orgName):
        return cls.query.filter_by(orgName=orgName).first()

    def save(self):
        db.session.add(self)
        db.session.commit()

    def delete(self):
        db.session.delete(self)
        db.session.commit()
    
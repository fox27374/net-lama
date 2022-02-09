from db.db import db

class SiteModel(db.Model):
    __tablename__ = 'sites'
    siteId = db.Column(db.Integer, primary_key=True)
    siteName = db.Column(db.String(80))

    def __init__(self, siteName):
        self.siteName = siteName

    def json(self):
        return {
            "siteId": self.siteId,
            "siteName": self.siteName
            }

    @classmethod
    def findById(cls, siteId):
        return cls.query.filter_by(siteId=siteId).first()

    @classmethod
    def findByName(cls, siteName):
        return cls.query.filter_by(siteName=siteName).first()

    def save(self):
        db.session.add(self)
        db.session.commit()

    def delete(self):
        db.session.delete(self)
        db.session.commit()
    
from flask_wtf import FlaskForm
from wtforms import StringField, SubmitField, HiddenField, BooleanField, SelectField
from wtforms.validators import DataRequired

class OrgAddForm(FlaskForm):
    orgName = StringField('Organization Name', validators=[DataRequired()])
    addSubmit = SubmitField('Submit')

class OrgDeleteForm(FlaskForm):
    orgId = HiddenField()
    delete = BooleanField('Are you sure?', default="unchecked", validators=[DataRequired()])
    deleteSubmit = SubmitField('Delete')

class OrgEditForm(FlaskForm):
    orgId = HiddenField()
    orgName = StringField('Organization Name', validators=[DataRequired()])
    editSubmit = SubmitField('Submit')


class SiteAddForm(FlaskForm):
    siteName = StringField('Site Name', validators=[DataRequired()])
    orgId = SelectField('Organization', choices=[], validate_choice=False)
    addSubmit = SubmitField('Submit')

class SiteDeleteForm(FlaskForm):
    siteId = HiddenField()
    delete = BooleanField('Are you sure?', default="unchecked", validators=[DataRequired()])
    deleteSubmit = SubmitField('Delete')

class SiteEditForm(FlaskForm):
    siteId = HiddenField()
    siteName = StringField('Site Name', validators=[DataRequired()])
    editSubmit = SubmitField('Submit')
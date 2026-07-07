from flask_wtf import FlaskForm
from wtforms import StringField, SubmitField, SelectField
from wtforms.validators import InputRequired, IPAddress, Length

class configForm(FlaskForm):
    sensorPiInterface = SelectField('Interface', choices=[('wlan0', 'wlan0'), ('wlan1', 'wlan1')])
    sensorPiChannels = StringField('Channels')
    mqttServer = StringField('MQTT Server', [InputRequired(), IPAddress(ipv4=True)])
    mqttPort = StringField('MQTT Server Port', [InputRequired()])
    splunkServer = StringField('Splunk Server', [IPAddress(ipv4=True)])
    splunkPort = StringField('Splunk Server Port')
    splunkToken = StringField('Splunk Token')
    submit = SubmitField('Submit')
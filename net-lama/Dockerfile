# ./Dockerfile

FROM python:3.8-alpine

# Labeling
LABEL "org.lnxnet.project"="net-lama"
LABEL "org.lnxnet.application"="net-lama"

# Set variables
ENV USER=net-lama
ENV GROUP=net-lama
ENV HOME /home/$USER

# Create user and group
RUN addgroup $GROUP
RUN adduser -G $GROUP -h $HOME -D $USER

# Use new directory and user
WORKDIR $HOME
USER $USER

# Copy source files
COPY --chown=$USER:$GROUP net-lama/configs configs
COPY --chown=$USER:$GROUP net-lama/db db
COPY --chown=$USER:$GROUP net-lama/models models
COPY --chown=$USER:$GROUP net-lama/resources resources
COPY --chown=$USER:$GROUP net-lama/schemas schemas
COPY --chown=$USER:$GROUP net-lama/requirements.txt .
COPY --chown=$USER:$GROUP net-lama/app.py .

# Set environment to the net-lama directory
ENV VIRTUAL_ENV=$HOME/venv
RUN python3 -m venv $VIRTUAL_ENV
ENV PATH="$VIRTUAL_ENV/bin:$PATH"

# Install dependencies:
RUN pip install --upgrade pip
RUN pip install --no-cache-dir -r requirements.txt

# Run the application:
CMD ["python", "app.py"]

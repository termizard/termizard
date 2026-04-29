FROM mcr.microsoft.com/windows/nanoserver:ltsc2025

ADD https://go.dev C:/go.msi
RUN msiexec.exe /i C:/go.msi /quiet /qn

ENV PATH="C:\go\bin;C:\Windows\system32;C:\Windows"

WORKDIR C:/app
COPY . .

RUN go build -o myterm-windows.exe ./cmd/main.go

CMD ["myterm-windows.exe"]

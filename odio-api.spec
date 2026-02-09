Name:           odio-api
Version:        0.3.3
Release:        2%{?dist}
Summary:        Web API for mpris, pulseaudio and systemd
License:        BSD-2-Clause
URL:            https://github.com/b0bbywan/go-odio-api
Source0:        https://github.com/b0bbywan/go-odio-api/archive/refs/tags/v%{version}.tar.gz

BuildRequires:  golang
BuildRequires:  git

%description
Web API for mpris, pulseaudio and systemd.

%prep
%setup -q -n go-odio-api-%{version}

%build
# Dossier temporaire pour le binaire
mkdir -p %{_tmppath}/bin
export GO111MODULE=on
go build -o %{_tmppath}/bin/odio-api github.com/b0bbywan/go-odio-api

%install
install -d %{buildroot}%{_bindir}
install -m 0755 %{_tmppath}/bin/odio-api %{buildroot}%{_bindir}/odio-api

install -d %{buildroot}%{_datadir}/odio-api
install -m 0644 share/config.yaml %{buildroot}%{_datadir}/odio-api/config.yaml

install -d %{buildroot}%{_userunitdir}
install -m 0644 debian/odio-api.service %{buildroot}%{_userunitdir}/odio-api.service

%files
%license LICENSE
%doc README.md debian/changelog
%{_bindir}/odio-api
%{_datadir}/odio-api/config.yaml
%{_userunitdir}/odio-api.service

%changelog
%autochangelog

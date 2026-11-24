nav = '''    <nav class="navbar navbar-expand navbar-dark bg-primary">
        <div class="container-fluid">
            <a class="navbar-brand" href="/"><strong>Pembukuan</strong></a>
            <ul class="navbar-nav ms-auto align-items-center">
                <li class="nav-item"><span class="nav-link text-white-50 small">{{.User.Username}}</span></li>
                {{if .User.IsSuperAdmin}}
                <li class="nav-item"><a class="nav-link small" href="/settings">Pengaturan</a></li>
                {{end}}
                <li class="nav-item"><a class="nav-link small" href="/logout">Keluar</a></li>
            </ul>
        </div>
    </nav>'''
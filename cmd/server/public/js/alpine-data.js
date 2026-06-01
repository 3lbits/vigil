document.addEventListener('alpine:init', () => {

    // Main app layout — sidebar + dark mode with $watch (arrow fns need registered component).
    Alpine.data('appLayout', () => ({
        sidebarOpen: false,
        darkMode: document.documentElement.classList.contains('dark'),
        init() {
            this.$watch('sidebarOpen', v => {
                if (v) this.$nextTick(() => this.$refs.closeBtn && this.$refs.closeBtn.focus());
            });
            this.$watch('darkMode', v => {
                document.documentElement.classList.toggle('dark', v);
                localStorage.setItem('darkMode', v);
            });
        }
    }));

    // Risk scoring panel (wizard step 3 — current risk).
    // initL/initC: initial likelihood/consequence from DB.
    // highMin: minimum score for red; lowMax: maximum score for green.
    Alpine.data('riskScorer', (initL, initC, highMin, lowMax) => ({
        l: initL,
        c: initC,
        score() { return this.l * this.c; },
        level(s) { return s >= highMin ? 'red' : s > lowMax ? 'yellow' : 'green'; },
        label(s) { return s >= highMin ? 'High' : s > lowMax ? 'Medium' : 'Low'; },
        bgClass(s) { return s >= highMin ? 'bg-red-500' : s > lowMax ? 'bg-amber-400' : 'bg-green-500'; }
    }));

    // Treatment scoring panel (wizard step 4 — target risk).
    Alpine.data('treatmentScorer', (initL, initC, highMin, lowMax) => ({
        tl: initL,
        tc: initC,
        label(s) { return s >= highMin ? 'High' : s > lowMax ? 'Medium' : 'Low'; },
        bgClass(s) { return s >= highMin ? 'bg-red-500' : s > lowMax ? 'bg-amber-400' : 'bg-green-500'; }
    }));

    // Activity completion toggle — $nextTick callback needs registered component.
    Alpine.data('activityComplete', () => ({
        showComplete: false,
        async toggle() {
            this.showComplete = !this.showComplete;
            if (this.showComplete) {
                await this.$nextTick();
                const el = document.getElementById('completed_by');
                if (el) el.focus();
            }
        }
    }));

});

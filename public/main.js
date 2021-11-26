(() => {
    function removeClass(klass) {
        Array.from(document.querySelectorAll("." + klass)).forEach((element) => {
            element.classList.remove(klass);
        });
    }

    Array.from(document.querySelectorAll(".tab-button")).forEach((button) => {
        button.addEventListener("click", () => {
            removeClass("tab-button-active");
            button.classList.add("tab-button-active");

            removeClass("tab-active");
            const id = button.getAttribute("data-tab-id");
            document.getElementById(id).classList.add("tab-active");
        });
    });
})();

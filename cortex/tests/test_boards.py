from cortex import boards


def test_recognises_big_boards_and_ignores_others():
    assert boards.recognised("https://www.amazon.jobs/en/search")
    assert boards.recognised("https://careers.oracle.com/jobs")
    assert boards.recognised("https://nvidia.wd5.myworkdayjobs.com/NVIDIAExternalCareerSite")
    assert not boards.recognised("https://example.com/careers")
    assert boards.search("https://example.com/careers", "x") is None


def test_clean_strips_markup():
    assert boards._clean("<b>Hi</b>  there\n you", 20) == "Hi there you"

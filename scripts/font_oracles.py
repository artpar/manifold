"""Original Go-font program hashes from pinned x/image v0.32.0, QA only.

Source: https://cs.opensource.google/go/x/image/+/refs/tags/v0.32.0:font/gofont/
Values were derived from the original module byte constants, not PDF output.
The package's Go font license is preserved in third_party/licenses/Go-fonts.txt.
"""
import hashlib

GO_FONT_SHA256 = {
 'GoRegular': '197d9f3703b4c00af609178876a8d73e396f64fe438b2c871778566632374be3',
 'Go-Bold': 'c18494baa7ea35b8dbfac3861787d360b9c0ab91232562371a09e6fbf09aaa40',
 'Go-Italic': '6efef00d080c8fe68bbe2bf08526294c83d52d7b90b16f938953025c0868cc57',
 'Go-BoldItalic': 'd6c33857c7ea7d740a8e1d81d37261782f698e9c17a2c9bf3bb0abbaf99e3c90',
 'GoMono': '8bc66a0154bbf69cd24e5bde41a12ec9495c8a242c5def2255e4a164900f1ed7',
 'GoMono-Bold': 'b165f1a142dc88d3c388de2c89d3ab62dd04620ef21ad07f43fb51f77857edc3',
 'GoMono-Italic': '10d5f73519ea32b4882897a9bec6c42ba2ca27d6c06ba9a0e55ae38a383cf7e6',
 'GoMono-BoldItalic': 'cb43aca5ab36631c0b4679831017cf249c33db4f721daad7379e04bba383ef1f',
}

def verify_go_program(name, data):
 assert name in GO_FONT_SHA256, name
 assert hashlib.sha256(data).hexdigest() == GO_FONT_SHA256[name], name
